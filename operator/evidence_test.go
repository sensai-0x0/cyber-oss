package operator

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/canonjson"
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
	got, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")+"\n"), 2, hashOf(t, lines[2]), action)
	if err != nil {
		t.Fatal(err)
	}
	if got.Seq != 2 || got.Digest != hashOf(t, lines[2]) || !strings.Contains(strings.Join(got.Lines, "\n"), "10.0.0.0/8") {
		t.Fatalf("evidence = %+v", got)
	}
}

func TestVerifyEvidenceRefusesATamperedEarlierRecord(t *testing.T) {
	action, lines := proposalFor(t)
	lines[0] = strings.Replace(lines[0], `"scorer_id":"s"`, `"scorer_id":"x"`, 1) // chain break behind the cited record
	_, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")), 2, hashOf(t, lines[2]), action)
	if !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v, want ErrEvidenceMismatch", err)
	}
}

func TestVerifyEvidenceRefusesTamperAfterTheInterrupt(t *testing.T) {
	action, lines := proposalFor(t)
	cited := hashOf(t, lines[2])
	lines[2] = strings.Replace(lines[2], `"approved":false`, `"approved":true`, 1)
	if _, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")), 2, cited, action); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestVerifyEvidenceRefusesTheWrongRecordOrAction(t *testing.T) {
	action, lines := proposalFor(t)
	in := func() *strings.Reader { return strings.NewReader(strings.Join(lines, "\n")) }
	cases := map[string]func() error{
		"wrong digest": func() error { _, e := VerifyEvidence(in(), 2, strings.Repeat("0", 64), action); return e },
		"other target": func() error {
			a := action
			a.Target = "10.1.0.0/16"
			_, e := VerifyEvidence(in(), 2, hashOf(t, lines[2]), a)
			return e
		},
		"other action id": func() error {
			a := action
			a.ID = "act-1"
			_, e := VerifyEvidence(in(), 2, hashOf(t, lines[2]), a)
			return e
		},
		"decision record, not a proposal": func() error { _, e := VerifyEvidence(in(), 1, hashOf(t, lines[1]), action); return e },
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
	spaced := strings.Replace(lines[0], `{"`, `{ "`, 1) + "\n" + lines[1]
	if _, err := VerifyEvidence(strings.NewReader(spaced+"\n"+lines[2]), 2, hashOf(t, lines[2]), action); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestApproveWithEvidenceSignsV2(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	action, lines := proposalFor(t)
	evidence := Evidence{Seq: 2, Digest: hashOf(t, lines[2])}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got, err := ApproveWithEvidence(action, private, "analyst", 5*time.Minute, now, testBreadth, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v2" || got.LedgerSeq != 2 || got.EvidenceDigest != evidence.Digest {
		t.Fatalf("action = %+v", got)
	}
	if err := enforcerd.VerifyActionSignature(public, got); err != nil {
		t.Fatal(err)
	}
	if _, err := ApproveWithEvidence(action, private, "analyst", time.Minute, now, testBreadth, Evidence{Seq: 1, Digest: "short"}); err == nil {
		t.Fatal("signed with a malformed digest")
	}
}

// Real records store evidence as a list of {key, value, class}, and values come from the target.
func TestVerifyEvidenceHandlesRealRecordShapeAndNeutralisesTerminalControls(t *testing.T) {
	record := map[string]any{
		"seq": 0, "prev_hash": genesis, "stage": "propose", "scorer_id": "s", "challengers": map[string]any{},
		"intel":    map[string]any{"confidence": 0.9, "rationale": []any{"observed jarm is on the pinned lab badlist"}},
		"evidence": []any{map[string]any{"key": "common_name", "value": "ok‮evil\u009b2J\x7f", "class": "OBSERVED"}},
		"policy":   map[string]any{"action": "mitigate", "action_id": "act-9", "kind": "nftables", "target": "198.51.100.9", "broad": false},
	}
	raw, _ := json.Marshal(record)
	body, err := canonjson.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := recordHash(body)
	if err != nil {
		t.Fatal(err)
	}
	record["record_hash"] = hash
	raw, _ = json.Marshal(record)
	line, _ := canonjson.Canonicalize(raw)
	action := enforcerd.Action{ID: "act-9", Kind: enforcerd.KindNFTables, Target: "198.51.100.9"}
	got, err := VerifyEvidence(strings.NewReader(string(line)), 0, hash, action)
	if err != nil {
		t.Fatalf("real-shaped record refused: %v", err)
	}
	shown := strings.Join(got.Lines, "\n")
	for _, raw := range []string{"‮", "\u009b", "\x7f", "\x1b"} {
		if strings.Contains(shown, raw) {
			t.Errorf("display line carries raw control %q", raw)
		}
	}
	if !strings.Contains(shown, `\u202e`) || !strings.Contains(shown, "common_name") {
		t.Fatalf("display = %q", shown)
	}
}

// realLine builds one chained record the way orchestrator/ledger.py does, for shapes the golden file lacks.
func realLine(t *testing.T, seq int64, previous string, record map[string]any) (string, string) {
	t.Helper()
	record["seq"], record["prev_hash"] = seq, previous
	raw, _ := json.Marshal(record)
	body, err := canonjson.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := recordHash(body)
	if err != nil {
		t.Fatal(err)
	}
	record["record_hash"] = hash
	raw, _ = json.Marshal(record)
	line, err := canonjson.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return string(line), hash
}

// The signature covers kind and broad, so the proposal must have proposed exactly those.
func TestVerifyEvidenceRefusesADifferentKindOrBreadth(t *testing.T) {
	action, lines := proposalFor(t)
	in := strings.Join(lines, "\n")
	otherKind := action
	otherKind.Kind = enforcerd.KindRPZ
	if _, err := VerifyEvidence(strings.NewReader(in), 2, hashOf(t, lines[2]), otherKind); !errors.Is(err, ErrEvidenceMismatch) {
		t.Errorf("other kind: err = %v, want ErrEvidenceMismatch", err)
	}
	otherBreadth := action
	otherBreadth.Broad = false
	if _, err := VerifyEvidence(strings.NewReader(in), 2, hashOf(t, lines[2]), otherBreadth); !errors.Is(err, ErrEvidenceMismatch) {
		t.Errorf("other broad: err = %v, want ErrEvidenceMismatch", err)
	}
}

// A proposal a later record already decided (approved or rejected) cannot be signed again.
func TestVerifyEvidenceRefusesAnAlreadyDecidedProposal(t *testing.T) {
	_, lines := proposalFor(t)
	decided := enforcerd.Action{ID: "act-1", Kind: enforcerd.KindNFTables, Target: "198.51.100.7"}
	if _, err := VerifyEvidence(strings.NewReader(strings.Join(lines, "\n")), 0, hashOf(t, lines[0]), decided); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v, want ErrEvidenceMismatch", err)
	}
	// The same proposal is signable while no decision follows it.
	if _, err := VerifyEvidence(strings.NewReader(lines[0]), 0, hashOf(t, lines[0]), decided); err != nil {
		t.Fatalf("undecided proposal refused: %v", err)
	}
}

// A later record that breaks the chain also refuses, since the whole export is read.
func TestVerifyEvidenceRefusesABreakAfterTheCitedRecord(t *testing.T) {
	_, lines := proposalFor(t)
	decided := enforcerd.Action{ID: "act-1", Kind: enforcerd.KindNFTables, Target: "198.51.100.7"}
	lines[2] = strings.Replace(lines[2], `"scorer_id":"s"`, `"scorer_id":"x"`, 1)
	if _, err := VerifyEvidence(strings.NewReader(lines[0]+"\n"+lines[2]), 0, hashOf(t, lines[0]), decided); !errors.Is(err, ErrEvidenceMismatch) {
		t.Fatalf("err = %v, want ErrEvidenceMismatch", err)
	}
}

func TestEvidenceProposalLineNeutralisesTerminalControls(t *testing.T) {
	target := "evil\x1b]52;c;aGk=\x07.example"
	line, hash := realLine(t, 0, genesis, map[string]any{
		"stage": "propose", "scorer_id": "s\u009b2J", "intel": nil, "evidence": []any{},
		"policy": map[string]any{"action": "mitigate", "action_id": "act-7", "kind": "rpz", "target": target, "broad": false},
	})
	got, err := VerifyEvidence(strings.NewReader(line), 0, hash, enforcerd.Action{ID: "act-7", Kind: enforcerd.KindRPZ, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	shown := strings.Join(got.Lines, "\n")
	for _, raw := range []string{"\x1b", "\x07", "\u009b"} {
		if strings.Contains(shown, raw) {
			t.Errorf("proposal line carries raw control %q", raw)
		}
	}
}
