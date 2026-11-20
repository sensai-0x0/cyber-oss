package enforcerd

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

var testBreadth = Breadth{V4: 24, V6: 64}

func TestDeriveBroadFromTargetAlone(t *testing.T) {
	cases := []struct {
		target string
		broad  bool
	}{
		{"10.0.0.0/8", true},
		{"192.0.2.0/24", true}, // boundary is inclusive
		{"192.0.2.0/25", false},
		{"192.0.2.7/32", false},
		{"192.0.2.7", false},
		{"2001:db8::/32", true},
		{"2001:db8::/64", true},
		{"2001:db8::/65", false},
		{"2001:db8::1", false},
		{"0.0.0.0/0", true},
		{"staging.example", false},
		{"::ffff:10.0.0.0/104", true}, // IPv4-mapped /8 is judged by IPv4 breadth
		{"::ffff:192.0.2.7/128", false},
	}
	for _, tc := range cases {
		got, err := testBreadth.Derive(tc.target)
		if err != nil || got != tc.broad {
			t.Errorf("Derive(%q) = %v, %v; want %v", tc.target, got, err, tc.broad)
		}
	}
}

func TestDeriveBroadRejectsUnparseableTargets(t *testing.T) {
	for _, target := range []string{"", "1.2.3.4/33", "http://10.0.0.0/8", "10.0.0.0/8 ", "-bad.example", "a b", "10.0.0.0/8;x", "fe80::1%eth0", "::ffff:10.0.0.0/64"} {
		if _, err := testBreadth.Derive(target); !errors.Is(err, ErrInvalidAction) {
			t.Errorf("Derive(%q) err = %v, want ErrInvalidAction", target, err)
		}
	}
}

func TestZeroBreadthFailsClosed(t *testing.T) {
	if _, err := (Breadth{}).Derive("10.0.0.0/8"); err == nil {
		t.Fatal("zero Breadth classified a target instead of failing closed")
	}
}

func TestParseBreadthReadsPolicyKeys(t *testing.T) {
	got, err := ParseBreadth(strings.NewReader("version: v1\nbroad_prefix_threshold: 24\nbroad_prefix_threshold_v6: 64\nauto_approve_narrow: false\n"))
	if err != nil || got != (Breadth{V4: 24, V6: 64}) {
		t.Fatalf("ParseBreadth = %+v, %v", got, err)
	}
	for _, bad := range []string{"", "broad_prefix_threshold: 24\n", "broad_prefix_threshold: x\nbroad_prefix_threshold_v6: 64\n", "broad_prefix_threshold: 99\nbroad_prefix_threshold_v6: 64\n"} {
		if _, err := ParseBreadth(strings.NewReader(bad)); err == nil {
			t.Errorf("ParseBreadth accepted %q", bad)
		}
	}
}

type recordingBackend struct{ applied int }

func (r *recordingBackend) Apply(context.Context, Action) error  { r.applied++; return nil }
func (r *recordingBackend) Revoke(context.Context, Action) error { return nil }

// SHR-001: a /8 signed with broad:false must not be installed.
func TestSignedBackendRejectsForgedNarrowFlag(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	backend := &recordingBackend{}
	signed := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth}
	action := Action{
		ID: "forged", Kind: KindNFTables, Target: "10.0.0.0/8", Broad: false, Approved: true,
		ApprovedBy: "analyst", TTL: time.Minute, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := SignAction(private, &action); err != nil {
		t.Fatal(err)
	}
	if err := signed.Apply(context.Background(), action); !errors.Is(err, ErrBroadMismatch) {
		t.Fatalf("Apply err = %v, want ErrBroadMismatch", err)
	}
	if backend.applied != 0 {
		t.Fatal("backend installed a forged-narrow /8")
	}
}

func TestSignedBackendWithoutBreadthFailsClosed(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	backend := &recordingBackend{}
	action := Action{ID: "a", Kind: KindRPZ, Target: "staging.example", Approved: true, TTL: time.Minute,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := SignAction(private, &action); err != nil {
		t.Fatal(err)
	}
	if err := (SignedBackend{PublicKey: public, Backend: backend}).Apply(context.Background(), action); err == nil || backend.applied != 0 {
		t.Fatalf("SignedBackend without Breadth applied: err=%v applied=%d", err, backend.applied)
	}
}

func TestSignedBackendAppliesCorrectlyClassifiedActions(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	backend := &recordingBackend{}
	signed := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth}
	for i, tc := range []struct {
		target string
		broad  bool
	}{{"10.0.0.0/8", true}, {"192.0.2.7", false}} {
		action := Action{ID: string(rune('a' + i)), Kind: KindNFTables, Target: tc.target, Broad: tc.broad,
			Approved: true, ApprovedBy: "analyst", TTL: time.Minute, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
		if err := SignAction(private, &action); err != nil {
			t.Fatal(err)
		}
		if err := signed.Apply(context.Background(), action); err != nil {
			t.Fatalf("%s: %v", tc.target, err)
		}
	}
	if backend.applied != 2 {
		t.Fatalf("applied = %d", backend.applied)
	}
}

// The Go and Python layers must read the same thresholds from one file.
func TestRepositoryPolicyYieldsBreadth(t *testing.T) {
	file, err := os.Open("../orchestrator/policy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	got, err := ParseBreadth(file)
	if err != nil || !got.valid() {
		t.Fatalf("ParseBreadth(policy.yaml) = %+v, %v", got, err)
	}
}

// Only top-level keys count, so Go reads the same thresholds Python's yaml.safe_load does.
func TestParseBreadthMatchesYAMLForNestedKeysCommentsAndDuplicates(t *testing.T) {
	nested := "version: v1\nbroad_prefix_threshold: 24\nbroad_prefix_threshold_v6: 64\noverrides:\n  broad_prefix_threshold: 8\n"
	if got, err := ParseBreadth(strings.NewReader(nested)); err != nil || got != (Breadth{V4: 24, V6: 64}) {
		t.Errorf("nested key: ParseBreadth = %+v, %v", got, err)
	}
	commented := "broad_prefix_threshold: 24  # ops\nbroad_prefix_threshold_v6: 64 # ops\n# broad_prefix_threshold: 8\n"
	if got, err := ParseBreadth(strings.NewReader(commented)); err != nil || got != (Breadth{V4: 24, V6: 64}) {
		t.Errorf("comments: ParseBreadth = %+v, %v", got, err)
	}
	duplicate := "broad_prefix_threshold: 24\nbroad_prefix_threshold_v6: 64\nbroad_prefix_threshold: 8\n"
	if _, err := ParseBreadth(strings.NewReader(duplicate)); err == nil {
		t.Error("duplicate key accepted")
	}
}

// YAML starts a comment only at a # preceded by whitespace, so "24#x" is a string, not 24.
func TestParseBreadthRefusesAHashWithoutSpaceBeforeIt(t *testing.T) {
	if _, err := ParseBreadth(strings.NewReader("broad_prefix_threshold: 24#x\nbroad_prefix_threshold_v6: 64\n")); err == nil {
		t.Fatal("read 24#x as 24")
	}
}
