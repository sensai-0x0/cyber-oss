package enforcerd

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestActionSignatureRoundTripAndTamperDetection(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	action := Action{
		ID: "signed", Kind: KindRPZ, Target: "staging.example", TTL: 5 * time.Minute,
		Approved: true, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := SignAction(private, &action); err != nil {
		t.Fatal(err)
	}
	if action.Signature == "" {
		t.Fatal("signature is empty")
	}
	if err := VerifyActionSignature(public, action); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	action.Target = "attacker-controlled.example"
	if err := VerifyActionSignature(public, action); err == nil {
		t.Fatal("tampered action passed signature verification")
	}
}

func TestVerifyActionSignatureRejectsMalformedSignature(t *testing.T) {
	public, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	action := Action{ID: "bad", Signature: base64.StdEncoding.EncodeToString([]byte("short"))}
	if err := VerifyActionSignature(public, action); err == nil {
		t.Fatal("malformed signature accepted")
	}
}

const goldenDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func v2Action() Action {
	return Action{
		Version: "v2", ID: "a1", Kind: KindNFTables, Target: "10.0.0.0/8", Broad: true, Approved: true,
		ApprovedBy: "analyst", TTL: 10 * time.Minute, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		EvidenceDigest: goldenDigest, LedgerSeq: 7,
	}
}

func TestV2CanonicalBytesMatchSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../schemas/mitigation_action.v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Actions []struct {
			Name   string `json:"name"`
			Action struct {
				ID         string    `json:"action_id"`
				Kind       Kind      `json:"kind"`
				Target     string    `json:"target"`
				Broad      bool      `json:"broad"`
				Approved   bool      `json:"approved"`
				ApprovedBy string    `json:"approved_by"`
				TTL        int64     `json:"ttl_seconds"`
				CreatedAt  time.Time `json:"created_at"`
				ExpiresAt  time.Time `json:"expires_at"`
				Digest     string    `json:"evidence_digest"`
				Seq        int64     `json:"ledger_seq"`
			} `json:"action"`
			Canonical string `json:"canonical"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Actions {
		a := Action{Version: "v2", ID: c.Action.ID, Kind: c.Action.Kind, Target: c.Action.Target, Broad: c.Action.Broad,
			Approved: c.Action.Approved, ApprovedBy: c.Action.ApprovedBy, TTL: time.Duration(c.Action.TTL) * time.Second,
			CreatedAt: c.Action.CreatedAt, ExpiresAt: c.Action.ExpiresAt, EvidenceDigest: c.Action.Digest, LedgerSeq: c.Action.Seq}
		got, err := canonicalAction(a)
		if err != nil || string(got) != c.Canonical {
			t.Errorf("%s:\n got %s (%v)\nwant %s", c.Name, got, err, c.Canonical)
		}
	}
}

func TestV1SignatureDoesNotVerifyAsV2AndBack(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	v1 := v2Action()
	v1.Version, v1.EvidenceDigest, v1.LedgerSeq = "", "", 0
	if err := SignAction(private, &v1); err != nil {
		t.Fatal(err)
	}
	if err := VerifyActionSignature(public, v1); err != nil {
		t.Fatal(err)
	}
	asV2 := v1
	asV2.Version, asV2.EvidenceDigest = "v2", goldenDigest
	if VerifyActionSignature(public, asV2) == nil {
		t.Fatal("a v1 signature verified as v2")
	}
	v2 := v2Action()
	if err := SignAction(private, &v2); err != nil {
		t.Fatal(err)
	}
	if err := VerifyActionSignature(public, v2); err != nil {
		t.Fatal(err)
	}
	asV1 := v2
	asV1.Version, asV1.EvidenceDigest, asV1.LedgerSeq = "", "", 0
	if VerifyActionSignature(public, asV1) == nil {
		t.Fatal("a v2 signature verified as v1")
	}
}

func TestV2SignatureCoversTheEvidence(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	action := v2Action()
	if err := SignAction(private, &action); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Action){
		"digest": func(a *Action) { a.EvidenceDigest = strings.Repeat("b", 64) },
		"seq":    func(a *Action) { a.LedgerSeq = 8 },
	} {
		tampered := action
		mutate(&tampered)
		if VerifyActionSignature(public, tampered) == nil {
			t.Errorf("tampered %s verified", name)
		}
	}
}

func TestV2RejectsMalformedEvidence(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(nil)
	for _, mutate := range []func(*Action){
		func(a *Action) { a.EvidenceDigest = "" },
		func(a *Action) { a.EvidenceDigest = strings.Repeat("A", 64) }, // upper case
		func(a *Action) { a.EvidenceDigest = "abc" },
		func(a *Action) { a.LedgerSeq = -1 },
		func(a *Action) { a.Version = "v3" },
	} {
		a := v2Action()
		mutate(&a)
		if SignAction(private, &a) == nil {
			t.Errorf("signed malformed action %+v", a)
		}
	}
	v1 := v2Action()
	v1.Version = ""
	if SignAction(private, &v1) == nil {
		t.Error("v1 action carrying evidence fields was signed")
	}
}

func TestMinVersionRefusesV1AfterCutOver(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	backend := &recordingBackend{}
	v1 := Action{ID: "old", Kind: KindRPZ, Target: "staging.example", Approved: true, TTL: time.Minute,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	v2 := v2Action()
	v2.Target, v2.Broad, v2.Kind = "192.0.2.7", false, KindNFTables
	for _, a := range []*Action{&v1, &v2} {
		if err := SignAction(private, a); err != nil {
			t.Fatal(err)
		}
	}
	window := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth, Now: fixedClock}
	if err := window.Apply(context.Background(), v1); err != nil {
		t.Fatalf("v1 refused during the window: %v", err)
	}
	cut := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth, MinVersion: "v2", Now: fixedClock}
	if err := cut.Apply(context.Background(), v1); !errors.Is(err, ErrVersionTooOld) {
		t.Fatalf("v1 after cut-over err = %v", err)
	}
	if err := cut.Apply(context.Background(), v2); err != nil {
		t.Fatalf("v2 refused after cut-over: %v", err)
	}
}

func TestUnknownMinVersionFailsClosed(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	backend := &recordingBackend{}
	a := Action{ID: "x", Kind: KindRPZ, Target: "staging.example", Approved: true, TTL: time.Minute,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := SignAction(private, &a); err != nil {
		t.Fatal(err)
	}
	signed := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth, MinVersion: "V2", Now: fixedClock}
	if signed.Apply(context.Background(), a) == nil || backend.applied != 0 {
		t.Fatal("a mistyped MinVersion let a v1 action through")
	}
}
