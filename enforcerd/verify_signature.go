package enforcerd

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SignAction signs the canonical action, including its approval and deadline.
func SignAction(privateKey ed25519.PrivateKey, action *Action) error {
	if len(privateKey) != ed25519.PrivateKeySize || action == nil {
		return errors.New("valid private key and action are required")
	}
	canonical, err := canonicalAction(*action)
	if err != nil {
		return err
	}
	action.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	return nil
}

// VerifyActionSignature fails closed on missing, malformed, or invalid signatures.
func VerifyActionSignature(publicKey ed25519.PublicKey, action Action) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return errors.New("invalid Ed25519 public key")
	}
	signature, err := base64.StdEncoding.DecodeString(action.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("invalid action signature encoding")
	}
	canonical, err := canonicalAction(action)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, canonical, signature) {
		return errors.New("action signature verification failed")
	}
	return nil
}

func canonicalAction(action Action) ([]byte, error) {
	if action.ID == "" || action.Target == "" || action.TTL <= 0 || action.TTL > MaxTTL {
		return nil, errors.New("invalid action identity or TTL")
	}
	if action.CreatedAt.IsZero() {
		return nil, errors.New("action creation time is required")
	}
	expiresAt := action.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = action.CreatedAt.Add(action.TTL)
	}
	if !expiresAt.Equal(action.CreatedAt.Add(action.TTL)) {
		return nil, errors.New("action expiry does not match TTL")
	}
	wire := struct {
		Version    string    `json:"schema_version"`
		ID         string    `json:"action_id"`
		Kind       Kind      `json:"kind"`
		Target     string    `json:"target"`
		Broad      bool      `json:"broad"`
		Approved   bool      `json:"approved"`
		ApprovedBy string    `json:"approved_by,omitempty"`
		TTLSeconds int64     `json:"ttl_seconds"`
		CreatedAt  time.Time `json:"created_at"`
		ExpiresAt  time.Time `json:"expires_at"`
	}{
		Version: "v1", ID: action.ID, Kind: action.Kind, Target: action.Target,
		Broad: action.Broad, Approved: action.Approved, ApprovedBy: action.ApprovedBy,
		TTLSeconds: int64(action.TTL / time.Second), CreatedAt: action.CreatedAt.UTC(), ExpiresAt: expiresAt.UTC(),
	}
	return json.Marshal(wire)
}

// SignedBackend verifies authorization before delegating to an RPZ/nft backend.
type SignedBackend struct {
	PublicKey ed25519.PublicKey
	Backend   Backend
}

func (s SignedBackend) Apply(ctx context.Context, action Action) error {
	if s.Backend == nil {
		return errors.New("signed backend is not configured")
	}
	if action.Broad && (!action.Approved || action.ApprovedBy == "") {
		return fmt.Errorf("%w: named operator approver is required", ErrApprovalRequired)
	}
	if err := VerifyActionSignature(s.PublicKey, action); err != nil {
		return err
	}
	return s.Backend.Apply(ctx, action)
}

func (s SignedBackend) Revoke(ctx context.Context, action Action) error {
	if s.Backend == nil {
		return errors.New("signed backend is not configured")
	}
	return s.Backend.Revoke(ctx, action)
}
