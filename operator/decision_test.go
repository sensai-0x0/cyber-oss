package operator

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

func TestApproveSetsOperatorExpiryAndSignature(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	action := enforcerd.Action{ID: "review", Kind: enforcerd.KindNFTables, Target: "192.0.2.0/24", Broad: true}
	got, err := Approve(action, private, "analyst", 5*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Approved || got.ApprovedBy != "analyst" || !got.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("approval = %+v", got)
	}
	if err := enforcerd.VerifyActionSignature(public, got); err != nil {
		t.Fatal(err)
	}
}

func TestRejectProducesNoSignedMitigation(t *testing.T) {
	_, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	action := enforcerd.Action{ID: "review", Kind: enforcerd.KindRPZ, Target: "a.example"}
	got, err := Reject(action, private, "analyst", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Approved || got.Signature != "" {
		t.Fatalf("rejected action was deployable: %+v", got)
	}
}
