package operator

import (
	"crypto/ed25519"
	"errors"
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
	got, err := Approve(action, private, "analyst", 5*time.Minute, now, testBreadth)
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

var testBreadth = enforcerd.Breadth{V4: 24, V6: 64}

// SHR-001: the operator signs a derived flag; a disagreeing claim is refused.
func TestApproveRefusesClaimedFlagThatDisagreesWithTarget(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(nil)
	forged := enforcerd.Action{ID: "f", Kind: enforcerd.KindNFTables, Target: "10.0.0.0/8", Broad: false}
	if _, err := Approve(forged, private, "analyst", time.Minute, time.Now(), testBreadth); !errors.Is(err, enforcerd.ErrBroadMismatch) {
		t.Fatalf("err = %v, want ErrBroadMismatch", err)
	}
	over := enforcerd.Action{ID: "o", Kind: enforcerd.KindNFTables, Target: "192.0.2.7", Broad: true}
	if _, err := Approve(over, private, "analyst", time.Minute, time.Now(), testBreadth); !errors.Is(err, enforcerd.ErrBroadMismatch) {
		t.Fatalf("over-claimed err = %v, want ErrBroadMismatch", err)
	}
}

func TestApproveRefusesUnparseableTarget(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(nil)
	action := enforcerd.Action{ID: "u", Kind: enforcerd.KindNFTables, Target: "not a target"}
	if _, err := Approve(action, private, "analyst", time.Minute, time.Now(), testBreadth); !errors.Is(err, enforcerd.ErrInvalidAction) {
		t.Fatalf("err = %v", err)
	}
}
