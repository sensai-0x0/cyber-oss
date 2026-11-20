// Package scope enforces I1-I5: every probe target must be checked against a signed,
// short-lived scope bundle before any dial. See schemas/scope_bundle.v1.json and
// plans/fleet/README.md Phase 1. Active probing outside lab/ is unauthorized until an
// Authorizer built from a verified Bundle gates it (ADR 0003 consequence 4).
package scope

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/metacogna/sentinel-hunter/canonjson"
)

// MaxWindow is the longest a bundle may stay valid without re-signing (design doc §9.1).
const MaxWindow = 24 * time.Hour

var (
	// ErrEmptyScope means the bundle named no targets. Refuse to start; never allow-all (I4).
	ErrEmptyScope = errors.New("scope: bundle has no targets")
	// ErrInvalidSignature covers a missing, malformed, unknown-key or failed-verify signature.
	ErrInvalidSignature = errors.New("scope: signature verification failed")
	// ErrWindowTooLong means not_after - not_before exceeds MaxWindow.
	ErrWindowTooLong = errors.New("scope: bundle window exceeds 24 hours")
	// ErrNotCurrent means now is outside [not_before, not_after).
	ErrNotCurrent = errors.New("scope: bundle is not valid at this time")
	// ErrEpochTooOld means scope_epoch is not newer than the highest epoch already accepted.
	ErrEpochTooOld = errors.New("scope: bundle epoch is not newer than the last seen epoch")
)

// RateLimits are the fleet-wide rate fields a bundle carries. hunterd/scope does not enforce
// them; the shared token bucket (I6) is fleet plan Phase 4.
type RateLimits struct {
	GlobalCPS       int
	PerBlockCPS     int
	PerIPPerMin     int
	PerWorkerMaxCPS int
}

// Bundle is the verified form of schemas/scope_bundle.v1.json. Construct one only through
// Parse; the zero value has an empty Targets list and must never be used to authorize a probe.
type Bundle struct {
	BundleID        string
	EngagementID    string
	ScopeEpoch      uint64
	NotBefore       time.Time
	NotAfter        time.Time
	AuthRef         string
	Targets         []string
	Exclusions      []string
	Ports           []int
	ProbeKinds      []string
	DomainWatchlist []string
	Rate            RateLimits
	KeyID           string
	Signature       string
}

type wireBundle struct {
	SchemaVersion string   `json:"schema_version"`
	BundleID      string   `json:"bundle_id"`
	EngagementID  string   `json:"engagement_id"`
	ScopeEpoch    uint64   `json:"scope_epoch"`
	NotBefore     string   `json:"not_before"`
	NotAfter      string   `json:"not_after"`
	AuthRef       string   `json:"auth_ref"`
	Targets       []string `json:"targets"`
	Exclusions    []string `json:"exclusions,omitempty"`
	Ports         []int    `json:"ports"`
	ProbeKinds    []string `json:"probe_kinds"`
	DomainWatch   []string `json:"domain_watchlist,omitempty"`
	Rate          struct {
		GlobalCPS       int `json:"global_cps"`
		PerBlockCPS     int `json:"per_block_cps"`
		PerIPPerMin     int `json:"per_ip_per_min"`
		PerWorkerMaxCPS int `json:"per_worker_max_cps"`
	} `json:"rate"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// Parse decodes, verifies and time/epoch-checks a scope bundle. now is the clock the window
// is checked against. minEpoch is the highest scope_epoch already accepted for this engagement
// (0 on first run); the caller is responsible for persisting the returned Bundle.ScopeEpoch
// across restarts. An empty, unsigned, expired, or epoch-stale bundle is refused, never
// defaulted to allow-all (I4).
func Parse(raw []byte, keys map[string]ed25519.PublicKey, now time.Time, minEpoch uint64) (Bundle, error) {
	if len(raw) == 0 {
		return Bundle{}, ErrEmptyScope
	}
	var wire wireBundle
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Bundle{}, fmt.Errorf("scope: decode bundle: %w", err)
	}
	if wire.SchemaVersion != "v1" {
		return Bundle{}, fmt.Errorf("scope: unknown schema_version %q", wire.SchemaVersion)
	}
	if len(wire.Targets) == 0 {
		return Bundle{}, ErrEmptyScope
	}
	if wire.Signature == "" || wire.KeyID == "" {
		return Bundle{}, ErrInvalidSignature
	}
	publicKey, known := keys[wire.KeyID]
	if !known {
		return Bundle{}, fmt.Errorf("%w: unknown key id %q", ErrInvalidSignature, wire.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(wire.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Bundle{}, ErrInvalidSignature
	}
	canonical, err := canonicalBundle(wire)
	if err != nil {
		return Bundle{}, err
	}
	if !ed25519.Verify(publicKey, canonical, signature) {
		return Bundle{}, ErrInvalidSignature
	}
	notBefore, err := time.Parse(time.RFC3339, wire.NotBefore)
	if err != nil {
		return Bundle{}, fmt.Errorf("scope: parse not_before: %w", err)
	}
	notAfter, err := time.Parse(time.RFC3339, wire.NotAfter)
	if err != nil {
		return Bundle{}, fmt.Errorf("scope: parse not_after: %w", err)
	}
	if !notAfter.After(notBefore) || notAfter.Sub(notBefore) > MaxWindow {
		return Bundle{}, ErrWindowTooLong
	}
	if now.Before(notBefore) || !now.Before(notAfter) {
		return Bundle{}, ErrNotCurrent
	}
	if wire.ScopeEpoch < minEpoch {
		return Bundle{}, ErrEpochTooOld
	}
	return Bundle{
		BundleID: wire.BundleID, EngagementID: wire.EngagementID, ScopeEpoch: wire.ScopeEpoch,
		NotBefore: notBefore.UTC(), NotAfter: notAfter.UTC(), AuthRef: wire.AuthRef,
		Targets: append([]string(nil), wire.Targets...), Exclusions: append([]string(nil), wire.Exclusions...),
		Ports: append([]int(nil), wire.Ports...), ProbeKinds: append([]string(nil), wire.ProbeKinds...),
		DomainWatchlist: append([]string(nil), wire.DomainWatch...),
		Rate: RateLimits{
			GlobalCPS: wire.Rate.GlobalCPS, PerBlockCPS: wire.Rate.PerBlockCPS,
			PerIPPerMin: wire.Rate.PerIPPerMin, PerWorkerMaxCPS: wire.Rate.PerWorkerMaxCPS,
		},
		KeyID: wire.KeyID, Signature: wire.Signature,
	}, nil
}

// Sign builds a wire bundle from b, computes its canonical signature over every field except
// Signature, and returns the signed JSON bytes. b.Signature is ignored on input. Used by the
// `operator scope sign` CLI and by tests; hunterd never signs, only verifies.
func Sign(privateKey ed25519.PrivateKey, keyID string, b Bundle) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("scope: invalid Ed25519 private key")
	}
	if len(b.Targets) == 0 {
		return nil, ErrEmptyScope
	}
	if keyID == "" {
		return nil, errors.New("scope: key id is required")
	}
	wire := wireBundle{
		SchemaVersion: "v1", BundleID: b.BundleID, EngagementID: b.EngagementID, ScopeEpoch: b.ScopeEpoch,
		NotBefore: b.NotBefore.UTC().Format(time.RFC3339), NotAfter: b.NotAfter.UTC().Format(time.RFC3339),
		AuthRef: b.AuthRef, Targets: b.Targets, Exclusions: b.Exclusions, Ports: b.Ports,
		ProbeKinds: b.ProbeKinds, DomainWatch: b.DomainWatchlist, KeyID: keyID,
	}
	wire.Rate.GlobalCPS = b.Rate.GlobalCPS
	wire.Rate.PerBlockCPS = b.Rate.PerBlockCPS
	wire.Rate.PerIPPerMin = b.Rate.PerIPPerMin
	wire.Rate.PerWorkerMaxCPS = b.Rate.PerWorkerMaxCPS
	canonical, err := canonicalBundle(wire)
	if err != nil {
		return nil, err
	}
	wire.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	return json.Marshal(wire)
}

// canonicalBundle returns the sorted-key canonical JSON of every field except Signature, so
// Sign and Parse hash exactly the same bytes (schema is Go-to-Go only; no cross-language
// parity requirement, unlike ADR 0005's v2 action).
func canonicalBundle(w wireBundle) ([]byte, error) {
	fields := map[string]any{
		"schema_version": "v1", "bundle_id": w.BundleID, "engagement_id": w.EngagementID,
		"scope_epoch": int64(w.ScopeEpoch), "not_before": w.NotBefore, "not_after": w.NotAfter,
		"auth_ref": w.AuthRef, "targets": toAny(w.Targets), "exclusions": toAny(w.Exclusions),
		"ports": toAnyInt(w.Ports), "probe_kinds": toAny(w.ProbeKinds), "domain_watchlist": toAny(w.DomainWatch),
		"rate": map[string]any{
			"global_cps": int64(w.Rate.GlobalCPS), "per_block_cps": int64(w.Rate.PerBlockCPS),
			"per_ip_per_min": int64(w.Rate.PerIPPerMin), "per_worker_max_cps": int64(w.Rate.PerWorkerMaxCPS),
		},
		"key_id": w.KeyID,
	}
	var out bytes.Buffer
	if err := canonjson.Write(&out, fields); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func toAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func toAnyInt(values []int) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = int64(v)
	}
	return out
}
