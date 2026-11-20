package scope

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/projectdiscovery/networkpolicy"
)

// Kind is a probe method named in a bundle's probe_kinds.
type Kind string

const (
	KindTLS  Kind = "tls"
	KindJARM Kind = "jarm"
	KindHTTP Kind = "http"
)

// ErrOutOfScope means the target, kind or port is not authorized. No packet may be sent.
var ErrOutOfScope = errors.New("scope: target is out of authorized scope")

// Resolver looks up a hostname's addresses. *net.Resolver satisfies it; tests inject a fake so
// no suite needs a real network (testing.md).
type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// Authorizer enforces I1-I5 before any dial. Build one with NewAuthorizer from a Bundle
// returned by Parse; Parse already refused an empty or unsigned bundle (I4).
//
// Authorization is decided entirely by this type's own CIDR and hostname matching, never by
// delegating the "what if nothing matches" default to a library: networkpolicy.NetworkPolicy
// treats an allow-list with zero CIDR entries as "no restriction" (allow every address), which
// would invert I4 for an engagement whose targets are hostnames only. NetworkPolicy is used
// solely as an optional L2 handle for fastdialer/httpx, and only once there is at least one
// CIDR entry to build it from.
type Authorizer struct {
	kinds     map[Kind]struct{}
	ports     map[int]struct{}
	cidrAllow []netip.Prefix
	cidrDeny  []netip.Prefix
	hostAllow []*regexp.Regexp
	hostDeny  []*regexp.Regexp
	resolver  Resolver
	netPolicy *networkpolicy.NetworkPolicy
}

// NewAuthorizer builds the allow/deny matchers from a verified bundle. resolver is the pinned
// resolver CheckHost uses to resolve hostnames; nil means net.DefaultResolver.
func NewAuthorizer(bundle Bundle, resolver Resolver) (*Authorizer, error) {
	if len(bundle.Targets) == 0 {
		return nil, ErrEmptyScope
	}
	cidrAllow, hostAllow, err := splitEntries(bundle.Targets)
	if err != nil {
		return nil, fmt.Errorf("scope: targets: %w", err)
	}
	cidrDeny, hostDeny, err := splitEntries(bundle.Exclusions)
	if err != nil {
		return nil, fmt.Errorf("scope: exclusions: %w", err)
	}
	kinds := make(map[Kind]struct{}, len(bundle.ProbeKinds))
	for _, k := range bundle.ProbeKinds {
		kinds[Kind(k)] = struct{}{}
	}
	ports := make(map[int]struct{}, len(bundle.Ports))
	for _, p := range bundle.Ports {
		ports[p] = struct{}{}
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	var netPolicy *networkpolicy.NetworkPolicy
	if len(cidrAllow) > 0 {
		netPolicy, err = networkpolicy.New(networkpolicy.Options{
			AllowList: prefixStrings(cidrAllow), DenyList: prefixStrings(cidrDeny), AllowPortList: bundle.Ports,
		})
		if err != nil {
			return nil, fmt.Errorf("scope: build L2 network policy: %w", err)
		}
	}
	return &Authorizer{
		kinds: kinds, ports: ports, cidrAllow: cidrAllow, cidrDeny: cidrDeny,
		hostAllow: hostAllow, hostDeny: hostDeny, resolver: resolver, netPolicy: netPolicy,
	}, nil
}

// splitEntries parses each entry as a CIDR or bare IP (promoted to /32 or /128), or else
// compiles it as an exact, case-insensitive, anchored hostname match. networkpolicy's own
// AllowList/DenyList treats a non-CIDR entry as an unanchored regular expression, so an
// unescaped hostname like "example.com" would also match "notexample.com.attacker.net" —
// widening scope beyond what the client authorized (I5). Anchoring here is this package's own
// matcher, not a networkpolicy option.
func splitEntries(entries []string) ([]netip.Prefix, []*regexp.Regexp, error) {
	var prefixes []netip.Prefix
	var hosts []*regexp.Regexp
	for _, entry := range entries {
		if prefix, ok := asPrefix(entry); ok {
			prefixes = append(prefixes, prefix)
			continue
		}
		pattern, err := regexp.Compile("^" + regexp.QuoteMeta(strings.ToLower(entry)) + "$")
		if err != nil {
			return nil, nil, fmt.Errorf("invalid hostname entry %q: %w", entry, err)
		}
		hosts = append(hosts, pattern)
	}
	return prefixes, hosts, nil
}

func asPrefix(entry string) (netip.Prefix, bool) {
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return prefix, true
	}
	if addr, err := netip.ParseAddr(entry); err == nil {
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		return netip.PrefixFrom(addr, bits), true
	}
	return netip.Prefix{}, false
}

func prefixStrings(prefixes []netip.Prefix) []string {
	out := make([]string, len(prefixes))
	for i, p := range prefixes {
		out[i] = p.String()
	}
	return out
}

// Check authorizes one already-resolved address and port for one probe kind (I1, I3). No
// packet may be sent when it returns a non-nil error.
func (a *Authorizer) Check(addr netip.Addr, port uint16, kind Kind) error {
	return a.authorize("", addr, port, kind)
}

// authorize is Check plus the hostname that resolved to addr, if any. A hostname match
// authorizes every address that hostname resolves to, independent of any CIDR allow entry, so
// a domain-only engagement (spec/SCOPE.md: "Exact CIDRs/domains") is not silently allow-all
// just because it has no CIDR entries.
func (a *Authorizer) authorize(host string, addr netip.Addr, port uint16, kind Kind) error {
	if _, ok := a.kinds[kind]; !ok {
		return fmt.Errorf("%w: probe kind %q is not authorized", ErrOutOfScope, kind)
	}
	if _, ok := a.ports[int(port)]; !ok {
		return fmt.Errorf("%w: port %d is not authorized", ErrOutOfScope, port)
	}
	addr = addr.Unmap()
	if host != "" && matchesHost(a.hostDeny, host) {
		return fmt.Errorf("%w: hostname %s is excluded", ErrOutOfScope, host)
	}
	if matchesPrefix(a.cidrDeny, addr) {
		return fmt.Errorf("%w: %s is excluded", ErrOutOfScope, addr)
	}
	if matchesPrefix(a.cidrAllow, addr) {
		return nil
	}
	if host != "" && matchesHost(a.hostAllow, host) {
		return nil
	}
	return fmt.Errorf("%w: %s:%d", ErrOutOfScope, addr, port)
}

func matchesPrefix(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func matchesHost(patterns []*regexp.Regexp, host string) bool {
	host = strings.ToLower(host)
	for _, p := range patterns {
		if p.MatchString(host) {
			return true
		}
	}
	return false
}

// CheckHost resolves host through the pinned resolver and requires every resolved address to
// be in scope: one out-of-scope IP rejects the whole target (I2). A literal IP (optionally
// bracketed, as "[::1]") is checked directly, with no resolution.
func (a *Authorizer) CheckHost(ctx context.Context, host string, port uint16, kind Kind) error {
	if addr, ok := parseLiteral(host); ok {
		return a.Check(addr, port, kind)
	}
	addrs, err := a.resolver.LookupHost(ctx, host)
	if err != nil {
		return fmt.Errorf("scope: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("%w: %s resolved to no addresses", ErrOutOfScope, host)
	}
	for _, raw := range addrs {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			return fmt.Errorf("scope: unparseable resolved address %q for %s: %w", raw, host, err)
		}
		if err := a.authorize(host, addr, port, kind); err != nil {
			return err
		}
	}
	return nil
}

func parseLiteral(host string) (netip.Addr, bool) {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	addr, err := netip.ParseAddr(host)
	return addr, err == nil
}

// OnValidateTarget adapts authorize to fastdialer.Options.OnValidateTarget (L2): the dialer
// calls it immediately before connect(2) on the real socket address, so it also catches DNS
// rebinding and any redirect the library follows internally, independent of the L1 check in
// CheckHost.
func (a *Authorizer) OnValidateTarget(kind Kind) func(hostname, ip, port string) error {
	return func(hostname, ip, port string) error {
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return fmt.Errorf("scope: unparseable dial address %q: %w", ip, err)
		}
		parsedPort, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return fmt.Errorf("scope: invalid port %q: %w", port, err)
		}
		return a.authorize(hostname, addr, uint16(parsedPort), kind)
	}
}

// NetworkPolicy returns the L2 handle for injection into fastdialer.Options.NetworkPolicy and
// httpx's runner.Options.Networkpolicy, for defense in depth on the library's own dial path
// (including internal redirects). It is nil when the bundle names no CIDR entry: a
// networkpolicy.NetworkPolicy built from hostname-only entries would not restrict addresses at
// all (see the Authorizer doc comment), so callers must fall back to L1 alone in that case.
func (a *Authorizer) NetworkPolicy() *networkpolicy.NetworkPolicy { return a.netPolicy }
