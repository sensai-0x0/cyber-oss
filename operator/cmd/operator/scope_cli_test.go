package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeygenRefusesToOverwriteExistingKey(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "key")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"keygen", "-out-prefix", prefix}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("first keygen: %v", err)
	}
	original, err := os.ReadFile(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"keygen", "-out-prefix", prefix}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("keygen silently overwrote an existing key")
	}
	after, err := os.ReadFile(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("the existing key was modified by the refused keygen")
	}
}

// scopeBundleJSON builds an unsigned bundle whose window is valid right now, so the test does
// not need updating as the real clock moves past a hardcoded date.
func scopeBundleJSON() string {
	now := time.Now().UTC()
	return fmt.Sprintf(`{
  "bundle_id": "sb_test", "engagement_id": "eng-test", "scope_epoch": 1,
  "not_before": %q, "not_after": %q,
  "auth_ref": "SOW-1", "targets": ["203.0.113.0/24"], "exclusions": [],
  "ports": [443], "probe_kinds": ["tls"],
  "rate": {"global_cps": 10, "per_block_cps": 5, "per_ip_per_min": 2, "per_worker_max_cps": 5}
}`, now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
}

func TestScopeSignThenVerifyRoundTrips(t *testing.T) {
	dir := t.TempDir()
	keyPrefix := filepath.Join(dir, "scope-key")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"keygen", "-out-prefix", keyPrefix}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run([]string{"scope", "sign", "-key", keyPrefix, "-key-id", "k1"}, strings.NewReader(scopeBundleJSON()), &stdout, &stderr); err != nil {
		t.Fatalf("sign: %v", err)
	}
	signed := stdout.String()
	if !strings.Contains(signed, `"key_id":"k1"`) || !strings.Contains(signed, `"signature":"`) {
		t.Fatalf("signed bundle missing key_id or signature: %s", signed)
	}
	var verifyOut bytes.Buffer
	err := run([]string{"scope", "verify", "-key", keyPrefix + ".pub", "-key-id", "k1"}, strings.NewReader(signed), &verifyOut, &stderr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(verifyOut.String(), "sb_test") {
		t.Fatalf("verify output = %q", verifyOut.String())
	}
}

func TestScopeVerifyRejectsTamperedBundle(t *testing.T) {
	dir := t.TempDir()
	keyPrefix := filepath.Join(dir, "scope-key")
	var stdout, stderr bytes.Buffer
	if err := run([]string{"keygen", "-out-prefix", keyPrefix}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := run([]string{"scope", "sign", "-key", keyPrefix, "-key-id", "k1"}, strings.NewReader(scopeBundleJSON()), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(stdout.String(), "203.0.113.0/24", "198.51.100.0/24", 1)
	var verifyOut bytes.Buffer
	err := run([]string{"scope", "verify", "-key", keyPrefix + ".pub", "-key-id", "k1"}, strings.NewReader(tampered), &verifyOut, &stderr)
	if err == nil {
		t.Fatal("verify accepted a tampered bundle")
	}
}

func TestScopeSignRequiresKeyID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run([]string{"scope", "sign", "-key", "/dev/null"}, strings.NewReader(scopeBundleJSON()), &stdout, &stderr)
	if err == nil {
		t.Fatal("sign accepted with no -key-id")
	}
}
