package probes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/projectdiscovery/httpx/runner"
	"github.com/projectdiscovery/networkpolicy"

	"github.com/metacogna/sentinel-hunter/hunterd/scope"
)

// HTTPRunner configures httpx for favicon hashing and page classification.
type HTTPRunner struct {
	timeout    time.Duration
	authorizer Authorizer
	resolver   scope.Resolver
}

// NewHTTPRunner builds a runner that checks authorizer before every dial (L1, I1-I2). resolver
// is used to pin the dial to one validated address for a hostname target (see Probe); nil means
// net.DefaultResolver. A nil authorizer is refused: there is no unauthorized-by-default path in
// this constructor.
func NewHTTPRunner(timeout time.Duration, authorizer Authorizer, resolver scope.Resolver) (*HTTPRunner, error) {
	if authorizer == nil {
		return nil, fmt.Errorf("http runner: an Authorizer is required (I1)")
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &HTTPRunner{timeout: timeout, authorizer: authorizer, resolver: resolver}, nil
}

// Probe performs an active HTTP request against one authorized target, after checking it
// against the Authorizer (L1, I1-I2).
//
// httpx resolves its InputTargetHost itself, independently of the check above, when it actually
// dials — a second, separate DNS lookup a hostile resolver could answer differently the second
// time (DNS rebinding), bypassing I2 for any bundle with no L2 NetworkPolicy (a domain-only
// bundle, which httpNetworkPolicy returns nil for). httpx exposes no per-dial hook to re-check
// at connect time the way TLSRunner's fastdialer OnValidateTarget does, so for a hostname target
// Probe instead resolves and checks every address itself, then pins the dial to one validated
// address via httpx's "vhost,ip" input form (CustomHost/Host in runner.go): the Host httpx then
// "resolves" is already a literal IP, so its internal lookup cannot diverge from what was
// checked here. A literal-IP target needs no pinning, since there is nothing left to resolve.
//
// Redirects are never followed (FollowRedirects is left at its zero value), so I2's "never
// follow a redirect off-scope" holds by construction. When authorizer also has an L2
// NetworkPolicy, it is injected as a second, independent check on httpx's own dial path.
func (r *HTTPRunner) Probe(ctx context.Context, target string) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	host, port, err := targetHostPort(target)
	if err != nil {
		return Observation{}, err
	}
	pinned, err := r.authorizeAndPin(ctx, host, port, target)
	if err != nil {
		return Observation{}, err
	}
	seconds := max(1, int(r.timeout.Round(time.Second)/time.Second))
	var (
		mu     sync.Mutex
		result *runner.Result
	)
	options := &runner.Options{
		InputTargetHost: []string{pinned}, Threads: 1, Timeout: seconds, Retries: 1,
		ExtractTitle: true, StatusCode: true, OutputIP: true, OutputServerHeader: true,
		Favicon: true, KnowledgeBase: true, DisableUpdateCheck: true, DisableStdout: true,
		DisableStdin: true, Silent: true, Networkpolicy: httpNetworkPolicy(r.authorizer),
		// SHR-017: OnResult can run on a different goroutine than the RunEnumeration caller;
		// result is read after close(done) below, so both sides take mu.
		OnResult: func(candidate runner.Result) {
			mu.Lock()
			defer mu.Unlock()
			if candidate.Err == nil && result == nil {
				found := candidate
				result = &found
			}
		},
	}
	client, err := runner.New(options)
	if err != nil {
		return Observation{}, fmt.Errorf("initialize httpx: %w", err)
	}
	defer client.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			client.Interrupt()
		case <-done:
		}
	}()
	client.RunEnumeration()
	close(done)
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	mu.Lock()
	found := result
	mu.Unlock()
	if found == nil {
		return Observation{}, errors.New("httpx returned no successful result")
	}
	return normalizeHTTPResponse(*found), nil
}

// authorizeAndPin checks host (and, for a hostname, every address it resolves to) against the
// Authorizer, and returns the httpx InputTargetHost value to dial. A literal IP is returned
// unchanged: there is no second resolution for httpx to diverge on. A hostname is resolved here
// (I2: every resolved address must be in scope), and the dial is pinned to the first validated
// address via "host,ip" (CustomHost,Host), dropping any scheme in target — httpx then probes
// both schemes against the pinned address, its own default behavior for a scheme-less target.
func (r *HTTPRunner) authorizeAndPin(ctx context.Context, host string, port uint16, target string) (string, error) {
	if _, ok := parseIPLiteral(host); ok {
		if err := r.authorizer.CheckHost(ctx, host, port, scope.KindHTTP); err != nil {
			return "", err
		}
		return target, nil
	}
	addrs, err := r.resolver.LookupHost(ctx, host)
	if err != nil {
		return "", fmt.Errorf("http runner: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("%w: %s resolved to no addresses", scope.ErrOutOfScope, host)
	}
	for _, addr := range addrs {
		if err := r.authorizer.CheckHost(ctx, addr, port, scope.KindHTTP); err != nil {
			return "", err
		}
	}
	portStr := strconv.FormatUint(uint64(port), 10)
	return net.JoinHostPort(host, portStr) + "," + net.JoinHostPort(addrs[0], portStr), nil
}

func parseIPLiteral(host string) (netip.Addr, bool) {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	addr, err := netip.ParseAddr(host)
	return addr, err == nil
}

// httpNetworkPolicy returns the L2 handle when authorizer is a *scope.Authorizer with a CIDR
// entry, and nil otherwise (a hostname-only bundle has no network policy to inject; L1 alone
// still gates every dial through authorizeAndPin above).
func httpNetworkPolicy(authorizer Authorizer) *networkpolicy.NetworkPolicy {
	a, ok := authorizer.(*scope.Authorizer)
	if !ok {
		return nil
	}
	return a.NetworkPolicy()
}

// targetHostPort splits an httpx InputTargetHost value (a bare host, host:port, or URL) into
// the host and port Probe checks against the Authorizer, defaulting to port 80, or 443 for an
// explicit https:// scheme, matching httpx's own default scheme behavior.
func targetHostPort(target string) (string, uint16, error) {
	raw := target
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", 0, fmt.Errorf("http runner: invalid target %q: %w", target, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", 0, fmt.Errorf("http runner: target %q has no host", target)
	}
	portStr := parsed.Port()
	if portStr == "" {
		portStr = "80"
		if parsed.Scheme == "https" {
			portStr = "443"
		}
	}
	port, err := scope.ParsePort(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func normalizeHTTPResponse(response runner.Result) Observation {
	attributes := map[string]any{
		"ip":          response.HostIP,
		"status_code": response.StatusCode,
	}
	if response.Title != "" {
		attributes["title"] = response.Title
	}
	if response.FavIconMMH3 != "" {
		attributes["favicon_mmh3"] = response.FavIconMMH3
	}
	if response.WebServer != "" {
		attributes["web_server"] = response.WebServer
	}
	if pageType, ok := response.KnowledgeBase["PageType"]; ok {
		attributes["page_type"] = pageType
	}
	return newObservation("httpx", response.URL, response.Timestamp, attributes)
}
