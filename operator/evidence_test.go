package operator

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

func goldenLedger(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../schemas/mitigation_action.v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Ledger []string `json:"ledger"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g.Ledger
}

func hashOf(t *testing.T, line string) string {
	t.Helper()
	var r struct {
		Hash string `json:"record_hash"`
	}
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatal(err)
	}
	return r.Hash
}

func proposalFor(t *testing.T) (enforcerd.Action, []string) {
	lines := goldenLedger(t)
	return enforcerd.Action{ID: "act-2", Kind: enforcerd.KindNFTables, Target: "10.0.0.0/8", Broad: true}, lines
}

func TestVerifyEvidenceAcceptsTheCitedProposal(t *testing.T) {
	action, lines := proposalFor(t)
	got, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")+"\n"), 1, hashOf(t, lines[1]), action)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 1 || got.Digest != hashOf(t, lines[1]) || !strings.Contains(strings.Join(got.Lines, "\n"), "10.0.0.0/8") {
		t.Fatalf("evidence = %+v", got)
	}
}

func TestVerifyEvidenceRefusesATamperedEarlierRecord(t *testing.T) {
	action, lines := proposalFor(t)
	lines[0] = strings.Replace(lines[0], `"scorer_id":"s"`, `"scorer_id":"x"`, 1) // chain break behind the cited record
	_, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")), 1, hashOf(t, lines[1]), action)
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v, want ErrEvidenceMismatch", err)
	}
}

func TestVerifyEvidenceRefusesTamperAfterTheInterrupt(t *testing.T) {
	action, lines := proposalFor(t)
	cited := hashOf(t, lines[1])
	lines[1] = strings.Replace(lines[1], `"approved":false`, `"approved":true`, 1)
	if _, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")), 1, cited, action); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyEvidenceRefusesTheWrongRecordOrAction(t *testing.T) {
	action, lines := proposalFor(t)
	in := func() *strings.Reader { return strings.NewReader(strings.Join(lines, "\n")) }
	cases := map[string]func() error{
		"wrong digest": func() error { _, e := VerifyEvidence(in(), 1, strings.Repeat("0", 64), action); return e },
		"other target": func() error {
			a := action
			a.Target = "10.1.0.0/16"
			_, e := VerifyEvidence(in(), 1, hashOf(t, lines[1]), a)
			return e
		},
		"other action id": func() error {
			a := action
			a.ID = "act-1"
			_, e := VerifyEvidence(in(), 1, hashOf(t, lines[1]), a)
			return e
		},
		"decision record, not a proposal": func() error { _, e := VerifyEvidence(in(), 2, hashOf(t, lines[2]), action); return e },
		"seq beyond the export":           func() error { _, e := VerifyEvidence(in(), 9, hashOf(t, lines[1]), action); return e },
		"negative seq":                    func() error { _, e := VerifyEvidence(in(), -1, hashOf(t, lines[1]), action); return e },
	}
	for name, run := range cases {
		if err := run(); !errors.Is(err, ErrEvidenceMismatch) {
			t.Errorf("%s: err = %v, want ErrEvidenceMismatch", name, err)
		}
	}
}

func TestVerifyEvidenceRefusesNonCanonicalExport(t *testing.T) {
	action, lines := proposalFor(t)
	spaced := strings.Replace(lines[0], `{"`, `{ "`, 1)
	if _, err := VerifyEvidence(strings.NewReader(spaced+"\n"+lines[1]), 1, hashOf(t, lines[1]), action); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestApproveWithEvidenceSignsV2(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	action, lines := proposalFor(t)
	evidence := Evidence{Seq: 1, Digest: hashOf(t, lines[1])}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := ApproveWithEvidence(action, private, "analyst", 5*time.Minute, now, testBreadth, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v2" || got.LedgerSeq != 1 || got.EvidenceDigest != evidence.Digest {
		t.Fatalf("action = %+v", got)
	}
	if err := enforcerd.VerifyActionSignature(public, got); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveWithEvidence(action, private, "analyst", time.Minute, now, testBreadth, Evidence{Seq: 1, Digest: "short"}); err == nil {
		t.Fatal("signed with a malformed digest")
	}
}
