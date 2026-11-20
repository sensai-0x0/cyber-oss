package enforcerd

import (
	"crypto/ed25519"
	"encoding/base64"
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
