package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

const policyYAML = "version: v1\nbroad_prefix_threshold: 24\nbroad_prefix_threshold_v6: 64\n"

type fixture struct {
	dir, key, policy, ledger string
	public                   ed25519.PublicKey
	lines                    []string
}

func setup(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	public, private, _ := ed25519.GenerateKey(nil)
	raw, err := os.ReadFile("../../../schemas/mitigation_action.v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Ledger []string `json:"ledger"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	f := fixture{dir: dir, key: filepath.Join(dir, "key"), policy: filepath.Join(dir, "policy.yaml"), ledger: filepath.Join(dir, "ledger.ndjson"), public: public, lines: g.Ledger}
	for path, data := range map[string][]byte{f.key: private, f.policy: []byte(policyYAML), f.ledger: []byte(strings.Join(g.Ledger, "\n") + "\n")} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func review(t *testing.T, f fixture, input string, extra ...string) (map[string]any, string, error) {
	t.Helper()
	args := append([]string{"review", "-key", f.key, "-decision", "approve", "-operator", "analyst", "-policy", f.policy}, extra...)
	var stdout, stderr bytes.Buffer
	err := run(args, strings.NewReader(input), &stdout, &stderr)
	var out map[string]any
	if err == nil {
		if e := json.Unmarshal(stdout.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
	}
	return out, stderr.String(), err
}

func digestOf(t *testing.T, line string) string {
	var r struct {
		H string `json:"record_hash"`
	}
	_ = json.Unmarshal([]byte(line), &r)
	return r.H
}

func TestReviewWithLedgerSignsV2AndShowsTheEvidence(t *testing.T) {
	f := setup(t)
	input := `{"action_id":"act-2","kind":"nftables","target":"10.0.0.0/8","broad":true,"ledger_seq":1,"evidence_digest":"` + digestOf(t, f.lines[1]) + `"}`
	out, shown, err := review(t, f, input, "-ledger", f.ledger)
	if err != nil {
		t.Fatal(err)
	}
	if out["schema_version"] != "v2" || out["ledger_seq"] != float64(1) || out["evidence_digest"] != digestOf(t, f.lines[1]) {
		t.Fatalf("output = %v", out)
	}
	if !strings.Contains(shown, "10.0.0.0/8") || !strings.Contains(shown, "ledger record 1") {
		t.Fatalf("operator was not shown the evidence: %q", shown)
	}
	action := enforcerd.Action{Version: "v2", ID: "act-2", Kind: enforcerd.KindNFTables, Target: "10.0.0.0/8", Broad: true, Approved: true,
		ApprovedBy: "analyst", EvidenceDigest: out["evidence_digest"].(string), LedgerSeq: 1}
	action.TTL = 15 * 60 * 1e9
	action.CreatedAt, _ = parseTime(out["created_at"].(string))
	action.ExpiresAt, _ = parseTime(out["expires_at"].(string))
	action.Signature = out["signature"].(string)
	if err := enforcerd.VerifyActionSignature(f.public, action); err != nil {
		t.Fatalf("signature: %v", err)
	}
}

func TestReviewRefusesWhenTheLedgerWasTampered(t *testing.T) {
	f := setup(t)
	cited := digestOf(t, f.lines[1])
	tampered := strings.Replace(strings.Join(f.lines, "\n"), `"scorer_id":"s"`, `"scorer_id":"x"`, 1)
	if err := os.WriteFile(f.ledger, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	input := `{"action_id":"act-2","kind":"nftables","target":"10.0.0.0/8","broad":true,"ledger_seq":1,"evidence_digest":"` + cited + `"}`
	if _, _, err := review(t, f, input, "-ledger", f.ledger); err == nil {
		t.Fatal("signed over a tampered ledger")
	}
}

func TestReviewNeverDropsCitedEvidenceSilently(t *testing.T) {
	f := setup(t)
	withCitation := `{"action_id":"act-2","kind":"nftables","target":"10.0.0.0/8","broad":true,"ledger_seq":1,"evidence_digest":"` + digestOf(t, f.lines[1]) + `"}`
	if _, _, err := review(t, f, withCitation); err == nil {
		t.Fatal("a cited action was signed as v1 without -ledger")
	}
	missing := `{"action_id":"act-2","kind":"nftables","target":"10.0.0.0/8","broad":true}`
	if _, _, err := review(t, f, missing, "-ledger", f.ledger); err == nil {
		t.Fatal("-ledger accepted an action with no citation")
	}
}

func TestReviewWithoutLedgerStaysV1AndWarns(t *testing.T) {
	f := setup(t)
	out, shown, err := review(t, f, `{"action_id":"a","kind":"rpz","target":"staging.example","broad":false}`)
	if err != nil || out["schema_version"] != "v1" || !strings.Contains(shown, "no evidence binding") {
		t.Fatalf("out=%v shown=%q err=%v", out, shown, err)
	}
}

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
