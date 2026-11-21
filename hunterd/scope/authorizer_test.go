package scope

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type fakeResolver map[string][]string

func (f fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	addrs, ok := f[host]
	if !ok {
		return nil, errors.New("fake resolver: no such host")
	}
	return addrs, nil
}

func newTestAuthorizer(t testing.TB, bundle Bundle, resolver Resolver) *Authorizer {
	t.Helper()
	a, err := NewAuthorizer(bundle, resolver)
	if err != nil {
		t.Fatalf("NewAuthorizer: %v", err)
	}
	return a
}

func cidrBundle() Bundle {
	return Bundle{
		Targets: []string{"203.0.113.0/24"}, Exclusions: []string{"203.0.113.128/27"},
		Ports: []int{443}, ProbeKinds: []string{"tls", "http"},
	}
}

func TestNewAuthorizerRejectsEmptyTargets(t *testing.T) {
	if _, err := NewAuthorizer(Bundle{}, nil); err != ErrEmptyScope {
		t.Fatalf("got %v, want ErrEmptyScope", err)
	}
}

func TestCheckRejectsUnauthorizedKind(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	addr := netip.MustParseAddr("203.0.113.5")
	if err := a.Check(addr, 443, KindJARM); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope", err)
	}
}

func TestCheckRejectsUnauthorizedPort(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	addr := netip.MustParseAddr("203.0.113.5")
	if err := a.Check(addr, 8443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope", err)
	}
}

func TestCheckAllowsInsideCIDRAndRejectsOutside(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	if err := a.Check(netip.MustParseAddr("203.0.113.5"), 443, KindTLS); err != nil {
		t.Fatalf("in-scope address rejected: %v", err)
	}
	if err := a.Check(netip.MustParseAddr("198.51.100.1"), 443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope for an address outside the CIDR", err)
	}
}

func TestCheckRejectsExclusion(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	// 203.0.113.128/27 is carved out of the broader 203.0.113.0/24 allow range (I5).
	if err := a.Check(netip.MustParseAddr("203.0.113.150"), 443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope for an excluded address inside the broader allow range", err)
	}
}

func TestCheckUnmapsIPv4MappedIPv6(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	addr := netip.MustParseAddr("::ffff:203.0.113.5")
	if err := a.Check(addr, 443, KindTLS); err != nil {
		t.Fatalf("IPv4-mapped IPv6 form of an in-scope address rejected: %v", err)
	}
}

func TestCheckHostLiteralIP(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	if err := a.CheckHost(context.Background(), "203.0.113.5", 443, KindTLS); err != nil {
		t.Fatalf("literal in-scope IP rejected: %v", err)
	}
	if err := a.CheckHost(context.Background(), "198.51.100.1", 443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope", err)
	}
}

// Regression: an exclusion written in IPv4-mapped-IPv6 notation must still deny the plain IPv4
// address it names (I5). Before unmapPrefix, "::ffff:203.0.113.150/128" built a /128 prefix in
// the 4-in-6 address family, which a plain "203.0.113.150" (the form a real dial or DNS answer
// produces) never matches, so the exclusion silently never fired.
func TestCheckRejectsIPv4MappedIPv6Exclusion(t *testing.T) {
	bundle := Bundle{
		Targets: []string{"203.0.113.0/24"}, Exclusions: []string{"::ffff:203.0.113.150/128"},
		Ports: []int{443}, ProbeKinds: []string{"tls"},
	}
	a := newTestAuthorizer(t, bundle, nil)
	if err := a.Check(netip.MustParseAddr("203.0.113.150"), 443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope: the 4-in-6 exclusion did not deny the plain IPv4 address", err)
	}
}

func TestCheckHostBracketedIPv6Literal(t *testing.T) {
	bundle := cidrBundle()
	bundle.Targets = []string{"2001:db8::/32"}
	a := newTestAuthorizer(t, bundle, nil)
	if err := a.CheckHost(context.Background(), "[2001:db8::1]", 443, KindTLS); err != nil {
		t.Fatalf("bracketed IPv6 literal rejected: %v", err)
	}
}

// One out-of-scope resolved IP rejects the whole hostname target (I2), even when another
// resolved IP for the same host is in scope.
func TestCheckHostRequiresEveryResolvedAddress(t *testing.T) {
	resolver := fakeResolver{"multi.example": {"203.0.113.5", "198.51.100.1"}}
	a := newTestAuthorizer(t, cidrBundle(), resolver)
	err := a.CheckHost(context.Background(), "multi.example", 443, KindTLS)
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope when one resolved address is out of scope", err)
	}
}

func TestCheckHostAcceptsWhenEveryResolvedAddressIsInScope(t *testing.T) {
	resolver := fakeResolver{"ok.example": {"203.0.113.5", "203.0.113.6"}}
	a := newTestAuthorizer(t, cidrBundle(), resolver)
	if err := a.CheckHost(context.Background(), "ok.example", 443, KindTLS); err != nil {
		t.Fatalf("all-in-scope host rejected: %v", err)
	}
}

func TestCheckHostRejectsWhenResolutionFails(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), fakeResolver{})
	if err := a.CheckHost(context.Background(), "nowhere.example", 443, KindTLS); err == nil {
		t.Fatal("resolution failure was not reported")
	}
}

// A domain-only engagement (spec/SCOPE.md: "Exact CIDRs/domains") has no CIDR entries at all.
// Its resolved addresses must be authorized by the hostname match, never by an accidental
// allow-all default (I4) just because cidrAllow is empty.
func TestHostnameOnlyBundleIsNotAllowAll(t *testing.T) {
	bundle := Bundle{Targets: []string{"client.example"}, Ports: []int{443}, ProbeKinds: []string{"tls"}}
	resolver := fakeResolver{
		"client.example":      {"203.0.113.5"},
		"unrelated.attacker.": {"198.51.100.1"},
	}
	a := newTestAuthorizer(t, bundle, resolver)
	if err := a.CheckHost(context.Background(), "client.example", 443, KindTLS); err != nil {
		t.Fatalf("authorized hostname rejected: %v", err)
	}
	if err := a.Check(netip.MustParseAddr("198.51.100.1"), 443, KindTLS); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope: an address with no hostname context and no CIDR match must never default to allowed", err)
	}
}

// Regression for the substring-match hazard in the underlying networkpolicy library: an
// unanchored hostname entry would also match a longer hostile hostname containing it as a
// substring (I5).
func TestHostnamePatternIsAnchoredNotSubstring(t *testing.T) {
	bundle := Bundle{Targets: []string{"example.com"}, Ports: []int{443}, ProbeKinds: []string{"tls"}}
	resolver := fakeResolver{"notexample.com.attacker.net": {"198.51.100.1"}}
	a := newTestAuthorizer(t, bundle, resolver)
	err := a.CheckHost(context.Background(), "notexample.com.attacker.net", 443, KindTLS)
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope: a substring match widened scope beyond the authorized hostname", err)
	}
}

func TestOnValidateTargetMatchesCheck(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	hook := a.OnValidateTarget(KindTLS)
	if err := hook("host.example", "203.0.113.5", "443"); err != nil {
		t.Fatalf("in-scope dial rejected: %v", err)
	}
	if err := hook("host.example", "198.51.100.1", "443"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("got %v, want ErrOutOfScope", err)
	}
}

func TestNetworkPolicyNilWithoutCIDREntries(t *testing.T) {
	bundle := Bundle{Targets: []string{"client.example"}, Ports: []int{443}, ProbeKinds: []string{"tls"}}
	a := newTestAuthorizer(t, bundle, fakeResolver{"client.example": {"203.0.113.5"}})
	if a.NetworkPolicy() != nil {
		t.Fatal("L2 handle must be nil for a hostname-only bundle, not a silently permissive policy")
	}
}

func TestNetworkPolicyPresentWithCIDREntries(t *testing.T) {
	a := newTestAuthorizer(t, cidrBundle(), nil)
	if a.NetworkPolicy() == nil {
		t.Fatal("L2 handle should be built when the bundle has a CIDR entry")
	}
}

func FuzzCheckHost(f *testing.F) {
	a := newTestAuthorizer(f, cidrBundle(), fakeResolver{"seed.example": {"203.0.113.5"}})
	f.Add("seed.example", uint16(443))
	f.Add("203.0.113.5", uint16(443))
	f.Add("", uint16(0))
	f.Fuzz(func(t *testing.T, host string, port uint16) {
		_ = a.CheckHost(context.Background(), host, port, KindTLS) // must not panic
	})
}
