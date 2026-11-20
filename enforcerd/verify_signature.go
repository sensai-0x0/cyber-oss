package enforcerd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/metacogna/sentinel-hunter/canonjson"
)

var evidenceDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// schemaVersion returns the signed schema version, treating the zero value as v1.
func (a Action) schemaVersion() string {
	if a.Version == "" {
		return "v1"
	}
	return a.Version
}

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
	switch action.schemaVersion() {
	case "v1":
		if action.EvidenceDigest != "" || action.LedgerSeq != 0 {
			return nil, errors.New("v1 action must not carry evidence fields")
		}
	case "v2":
		if !evidenceDigest.MatchString(action.EvidenceDigest) || action.LedgerSeq < 0 {
			return nil, errors.New("v2 action needs a lowercase SHA-256 evidence digest and a ledger sequence")
		}
		return canonicalV2(action, expiresAt.UTC())
	default:
		return nil, fmt.Errorf("unknown action version %q", action.Version)
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

// canonicalV2 is the v1 field list in the same order plus the evidence citation. The version
// is signed, so a v1 signature cannot verify as v2 or the reverse.
func canonicalV2(action Action, expiresAt time.Time) ([]byte, error) {
	fields := canonjson.Object{
		{Key: "schema_version", Value: "v2"}, {Key: "action_id", Value: action.ID},
		{Key: "kind", Value: string(action.Kind)}, {Key: "target", Value: action.Target},
		{Key: "broad", Value: action.Broad}, {Key: "approved", Value: action.Approved},
	}
	if action.ApprovedBy != "" {
		fields = append(fields, canonjson.Member{Key: "approved_by", Value: action.ApprovedBy})
	}
	fields = append(fields,
		canonjson.Member{Key: "ttl_seconds", Value: int64(action.TTL / time.Second)},
		canonjson.Member{Key: "created_at", Value: action.CreatedAt.UTC().Format(time.RFC3339Nano)},
		canonjson.Member{Key: "expires_at", Value: expiresAt.Format(time.RFC3339Nano)},
		canonjson.Member{Key: "evidence_digest", Value: action.EvidenceDigest},
		canonjson.Member{Key: "ledger_seq", Value: action.LedgerSeq},
	)
	var out bytes.Buffer
	if err := canonjson.Write(&out, fields); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// SignedBackend verifies authorization before delegating to an RPZ/nft backend.
type SignedBackend struct {
	PublicKey ed25519.PublicKey
	Backend   Backend
	// Breadth is required. Broad is derived from the target here, never trusted from the caller (SHR-001).
	Breadth Breadth
	// MinVersion is the oldest accepted schema version. Empty means "v1" during the ADR 0005
	// compatibility window; set "v2" at the cut-over.
	MinVersion string
	// Now is the clock expiry is checked against. Nil means time.Now; tests pin it.
	Now func() time.Time
}

func (s SignedBackend) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s SignedBackend) Apply(ctx context.Context, action Action) error {
	if s.Backend == nil {
		return errors.New("signed backend is not configured")
	}
	switch s.MinVersion {
	case "", "v1", "v2":
	default:
		return fmt.Errorf("signed backend: unknown MinVersion %q", s.MinVersion) // fail closed on a typo
	}
	if s.MinVersion == "v2" && action.schemaVersion() != "v2" {
		return fmt.Errorf("%w: need v2, got %s", ErrVersionTooOld, action.schemaVersion())
	}
	if err := ValidateTTL(action.TTL); err != nil {
		return err
	}
	// A valid signature does not make a stale or future-dated action live (SHR-002).
	if _, err := deadline(action, s.now()); err != nil {
		return err
	}
	if err := s.Breadth.Check(action.Target, action.Broad); err != nil {
		return err
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
