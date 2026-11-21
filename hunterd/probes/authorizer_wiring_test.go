package probes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/hunterd/scope"
)

// fakeAuthorizer lets probe-wiring tests assert the scope check runs before any dial, without
// depending on hunterd/scope's own matching logic (that is scope's own test suite's job).
type fakeAuthorizer struct {
	err error
	// hosts, ports and kinds record every CheckHost call, so a test can assert the exact
	// target the runner was about to dial, not just that some check happened.
	calls []checkCall
}

type checkCall struct {
	host string
	port uint16
	kind scope.Kind
}

func (f *fakeAuthorizer) CheckHost(_ context.Context, host string, port uint16, kind scope.Kind) error {
	f.calls = append(f.calls, checkCall{host, port, kind})
	return f.err
}

// fakeHostResolver lets HTTPRunner wiring tests exercise the resolve-every-address path with
// no real network (testing.md).
type fakeHostResolver map[string][]string

func (f fakeHostResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	addrs, ok := f[host]
	if !ok {
		return nil, errors.New("fake resolver: no such host")
	}
	return addrs, nil
}

func TestNewTLSRunnerRejectsNilAuthorizer(t *testing.T) {
	if _, err := NewTLSRunner(time.Second, nil); err == nil {
		t.Fatal("nil Authorizer was accepted")
	}
}

func TestNewHTTPRunnerRejectsNilAuthorizer(t *testing.T) {
	if _, err := NewHTTPRunner(time.Second, nil, nil); err == nil {
		t.Fatal("nil Authorizer was accepted")
	}
}

func TestTLSProbeChecksScopeBeforeDialing(t *testing.T) {
	denied := &fakeAuthorizer{err: scope.ErrOutOfScope}
	runner, err := NewTLSRunner(time.Second, denied)
	if err != nil {
		t.Fatal(err)
	}
	// 203.0.113.0/24 (TEST-NET-3, RFC 5737) never answers, so a real dial attempt here would
	// hang or fail slowly; an immediate error proves the check ran first.
	_, probeErr := runner.Probe(context.Background(), "host.example", "203.0.113.1", "443")
	if !errors.Is(probeErr, scope.ErrOutOfScope) {
		t.Fatalf("got %v, want scope.ErrOutOfScope", probeErr)
	}
	if len(denied.calls) != 1 || denied.calls[0].host != "203.0.113.1" || denied.calls[0].port != 443 || denied.calls[0].kind != scope.KindTLS {
		t.Fatalf("unexpected calls: %+v", denied.calls)
	}
}

func TestTLSProbeChecksHostWhenIPIsEmpty(t *testing.T) {
	denied := &fakeAuthorizer{err: scope.ErrOutOfScope}
	runner, err := NewTLSRunner(time.Second, denied)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = runner.Probe(context.Background(), "host.example", "", "443")
	if len(denied.calls) != 1 || denied.calls[0].host != "host.example" {
		t.Fatalf("expected a check against host.example, got %+v", denied.calls)
	}
}

func TestHTTPProbeChecksScopeBeforeDialing(t *testing.T) {
	denied := &fakeAuthorizer{err: scope.ErrOutOfScope}
	runner, err := NewHTTPRunner(time.Second, denied, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, probeErr := runner.Probe(context.Background(), "203.0.113.1:8443")
	if !errors.Is(probeErr, scope.ErrOutOfScope) {
		t.Fatalf("got %v, want scope.ErrOutOfScope", probeErr)
	}
	if len(denied.calls) != 1 || denied.calls[0].host != "203.0.113.1" || denied.calls[0].port != 8443 || denied.calls[0].kind != scope.KindHTTP {
		t.Fatalf("unexpected calls: %+v", denied.calls)
	}
}

// Regression for the DNS-rebinding gap: httpx would otherwise resolve host.example itself,
// independently of this check, when it actually dials. Probe must resolve and check every
// address itself (I2) before building the pinned target, so a second, different DNS answer at
// httpx's own dial time is never possible.
func TestHTTPProbeResolvesAndChecksEveryAddress(t *testing.T) {
	denied := &fakeAuthorizer{err: scope.ErrOutOfScope}
	resolver := fakeHostResolver{"host.example": {"203.0.113.1", "203.0.113.2"}}
	runner, err := NewHTTPRunner(time.Second, denied, resolver)
	if err != nil {
		t.Fatal(err)
	}
	_, probeErr := runner.Probe(context.Background(), "host.example:8443")
	if !errors.Is(probeErr, scope.ErrOutOfScope) {
		t.Fatalf("got %v, want scope.ErrOutOfScope", probeErr)
	}
	if len(denied.calls) != 1 || denied.calls[0].host != "203.0.113.1" || denied.calls[0].port != 8443 {
		t.Fatalf("expected the check against the first resolved address, got %+v", denied.calls)
	}
}

func TestHTTPProbePinsToTheFirstValidatedAddress(t *testing.T) {
	allowed := &fakeAuthorizer{}
	resolver := fakeHostResolver{"host.example": {"203.0.113.1", "203.0.113.2"}}
	runner, err := NewHTTPRunner(time.Second, allowed, resolver)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := runner.authorizeAndPin(context.Background(), "host.example", 8443, "host.example:8443")
	if err != nil {
		t.Fatal(err)
	}
	const want = "host.example:8443,203.0.113.1:8443"
	if pinned != want {
		t.Fatalf("got %q, want %q", pinned, want)
	}
	if len(allowed.calls) != 2 || allowed.calls[0].host != "203.0.113.1" || allowed.calls[1].host != "203.0.113.2" {
		t.Fatalf("expected both resolved addresses checked, got %+v", allowed.calls)
	}
}

func TestHTTPProbeRejectsWhenResolutionFails(t *testing.T) {
	allowed := &fakeAuthorizer{}
	runner, err := NewHTTPRunner(time.Second, allowed, fakeHostResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.authorizeAndPin(context.Background(), "nowhere.example", 443, "nowhere.example"); err == nil {
		t.Fatal("resolution failure was not reported")
	}
}

func TestHTTPProbeLiteralIPSkipsResolution(t *testing.T) {
	allowed := &fakeAuthorizer{}
	// No resolver entry for "203.0.113.1": a literal IP must never reach the resolver.
	runner, err := NewHTTPRunner(time.Second, allowed, fakeHostResolver{})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := runner.authorizeAndPin(context.Background(), "203.0.113.1", 443, "203.0.113.1:443")
	if err != nil {
		t.Fatal(err)
	}
	if pinned != "203.0.113.1:443" {
		t.Fatalf("literal IP target was rewritten: %q", pinned)
	}
}

func TestTargetHostPortParsesVariousForms(t *testing.T) {
	cases := []struct {
		target   string
		wantHost string
		wantPort uint16
	}{
		{"example.com", "example.com", 80},
		{"example.com:8080", "example.com", 8080},
		{"http://example.com", "example.com", 80},
		{"https://example.com", "example.com", 443},
		{"https://example.com:9443/path", "example.com", 9443},
		{"203.0.113.1", "203.0.113.1", 80},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			host, port, err := targetHostPort(c.target)
			if err != nil {
				t.Fatal(err)
			}
			if host != c.wantHost || port != c.wantPort {
				t.Fatalf("got (%s, %d), want (%s, %d)", host, port, c.wantHost, c.wantPort)
			}
		})
	}
}

func TestTargetHostPortRejectsHostless(t *testing.T) {
	if _, _, err := targetHostPort("http://"); err == nil {
		t.Fatal("hostless target accepted")
	}
}
