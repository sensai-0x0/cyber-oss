package scope

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func testBundle(now time.Time) Bundle {
	return Bundle{
		BundleID: "sb_test", EngagementID: "eng-test", ScopeEpoch: 1,
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		AuthRef: "SOW-1", Targets: []string{"203.0.113.0/24"}, Exclusions: []string{"203.0.113.128/27"},
		Ports: []int{443, 8443}, ProbeKinds: []string{"tls", "http"},
		Rate: RateLimits{GlobalCPS: 200, PerBlockCPS: 5, PerIPPerMin: 12, PerWorkerMaxCPS: 20},
	}
}

func TestParseRejectsEmptyBytes(t *testing.T) {
	if _, err := Parse(nil, nil, time.Now(), 0); err != ErrEmptyScope {
		t.Fatalf("got %v, want ErrEmptyScope", err)
	}
	if _, err := Parse([]byte{}, nil, time.Now(), 0); err != ErrEmptyScope {
		t.Fatalf("got %v, want ErrEmptyScope", err)
	}
}

func TestSignThenParseRoundTrips(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	b := testBundle(now)
	raw, err := Sign(private, "k1", b)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"k1": public}
	parsed, err := Parse(raw, keys, now, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.BundleID != b.BundleID || parsed.KeyID != "k1" || len(parsed.Targets) != 1 {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed.Signature == "" {
		t.Fatal("signature not carried through")
	}
}

func TestParseRejectsUnknownKeyID(t *testing.T) {
	_, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	raw, err := Sign(private, "k1", testBundle(now))
	if err != nil {
		t.Fatal(err)
	}
	other, _ := testKey(t)
	if _, err := Parse(raw, map[string]ed25519.PublicKey{"k2": other}, now, 0); err == nil {
		t.Fatal("verified against the wrong key id")
	}
}

func TestParseRejectsTamperedBody(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	raw, err := Sign(private, "k1", testBundle(now))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"203.0.113.0/24"`, `"198.51.100.0/24"`, 1)
	if tampered == string(raw) {
		t.Fatal("fixture did not change")
	}
	if _, err := Parse([]byte(tampered), map[string]ed25519.PublicKey{"k1": public}, now, 0); err != ErrInvalidSignature {
		t.Fatalf("got %v, want ErrInvalidSignature", err)
	}
}

func TestParseRejectsMalformedSignatureEncoding(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	raw, err := Sign(private, "k1", testBundle(now))
	if err != nil {
		t.Fatal(err)
	}
	validSig := extractField(t, raw, "signature")
	tampered := strings.Replace(string(raw), validSig, "not-base64!!", 1)
	if _, err := Parse([]byte(tampered), map[string]ed25519.PublicKey{"k1": public}, now, 0); err != ErrInvalidSignature {
		t.Fatalf("got %v, want ErrInvalidSignature", err)
	}
}

func TestParseRejectsWindowOver24Hours(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	b := testBundle(now)
	b.NotBefore = now.Add(-time.Minute)
	b.NotAfter = b.NotBefore.Add(25 * time.Hour)
	raw, err := Sign(private, "k1", b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw, map[string]ed25519.PublicKey{"k1": public}, now, 0); err != ErrWindowTooLong {
		t.Fatalf("got %v, want ErrWindowTooLong", err)
	}
}

func TestParseRejectsOutsideWindow(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	b := testBundle(now)
	raw, err := Sign(private, "k1", b)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"k1": public}
	cases := map[string]time.Time{
		"before not_before": b.NotBefore.Add(-time.Second),
		"at not_after":      b.NotAfter,
		"after not_after":   b.NotAfter.Add(time.Second),
	}
	for name, at := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(raw, keys, at, 0); err != ErrNotCurrent {
				t.Fatalf("got %v, want ErrNotCurrent", err)
			}
		})
	}
}

func TestParseRejectsLowerEpochButAcceptsEqual(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	b := testBundle(now)
	b.ScopeEpoch = 5
	raw, err := Sign(private, "k1", b)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"k1": public}
	if _, err := Parse(raw, keys, now, 6); err != ErrEpochTooOld {
		t.Fatalf("got %v, want ErrEpochTooOld for a lower epoch", err)
	}
	if _, err := Parse(raw, keys, now, 5); err != nil {
		t.Fatalf("an equal epoch must be accepted: %v", err)
	}
}

func TestSignRejectsEmptyTargets(t *testing.T) {
	_, private := testKey(t)
	b := testBundle(time.Now())
	b.Targets = nil
	if _, err := Sign(private, "k1", b); err != ErrEmptyScope {
		t.Fatalf("got %v, want ErrEmptyScope", err)
	}
}

// extractField returns the quoted value of a top-level string field, for building a tampered
// fixture without hand-writing JSON offsets.
func extractField(t *testing.T, raw []byte, field string) string {
	t.Helper()
	marker := `"` + field + `":"`
	s := string(raw)
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatalf("field %q not found in %s", field, s)
	}
	start += len(marker)
	end := strings.Index(s[start:], `"`)
	if end < 0 {
		t.Fatalf("field %q value not terminated", field)
	}
	return s[start : start+end]
}

func TestFuzzParseNeverPanics(t *testing.T) {
	public, private := testKey(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	valid, err := Sign(private, "k1", testBundle(now))
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{"k1": public}
	inputs := [][]byte{nil, {}, []byte("{"), []byte("null"), []byte(`{"schema_version":"v1"}`), valid}
	for i := 0; i < len(valid); i += 7 {
		mutated := append([]byte(nil), valid...)
		mutated[i] ^= 0xFF
		inputs = append(inputs, mutated)
	}
	for _, in := range inputs {
		_, _ = Parse(in, keys, now, 0) // must not panic regardless of the error returned
	}
}

func TestSignDeterministicCanonicalForm(t *testing.T) {
	// Pins the canonical byte form with a fixed seed so a future refactor that silently
	// reorders or reformats a signed field is caught by a changed signature, not just by a
	// round trip that would still pass against itself.
	seed := make([]byte, ed25519.SeedSize)
	private := ed25519.NewKeyFromSeed(seed)
	b := Bundle{
		BundleID: "sb_pin", EngagementID: "eng-pin", ScopeEpoch: 7,
		NotBefore: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		AuthRef:   "SOW-1", Targets: []string{"203.0.113.0/24"}, Ports: []int{443},
		ProbeKinds: []string{"tls"}, Rate: RateLimits{GlobalCPS: 1, PerBlockCPS: 1, PerIPPerMin: 1, PerWorkerMaxCPS: 1},
	}
	raw, err := Sign(private, "k1", b)
	if err != nil {
		t.Fatal(err)
	}
	const wantSignature = "PBqNSHtfV2BY+qCSeB1xArhK+H5lPqBnUppVpN3IwOSULypKxx/ddxpQaLiBFwsKmEuctnu046mUJ/KocR5UCA=="
	got := extractField(t, raw, "signature")
	if got != wantSignature {
		t.Fatalf("canonical form changed: got signature %s, want %s", got, wantSignature)
	}
}
