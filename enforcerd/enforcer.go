// Package enforcerd enforces approved, reversible actions with mandatory TTLs.
package enforcerd

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrApprovalRequired = errors.New("operator approval required for broad action")
	ErrInvalidTTL       = errors.New("TTL must be greater than zero and no more than 24 hours")
	ErrDuplicateAction  = errors.New("mitigation action already exists")
	ErrInvalidAction    = errors.New("invalid mitigation action")
	ErrVersionTooOld    = errors.New("action schema version is below the enforcer minimum")
	ErrActionExpired    = errors.New("action is past its deadline")
)

type Kind string

const (
	KindNFTables  Kind = "nftables"
	KindRPZ       Kind = "rpz"
	KindHeadscale Kind = "headscale"
)

// Action is backend-neutral; Broad actions must be explicitly approved.
type Action struct {
	// Version is the signed schema version: "" or "v1", or "v2" (ADR 0005).
	Version string
	// EvidenceDigest and LedgerSeq cite the ledger record the operator reviewed. v2 only.
	EvidenceDigest string
	LedgerSeq      int64
	ID             string
	Kind           Kind
	Target         string
	TTL            time.Duration
	Broad          bool
	Approved       bool
	ApprovedBy     string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	Signature      string
}

type Receipt struct {
	ActionID  string
	ExpiresAt time.Time
}

// Backend is implemented by nftables, DNS RPZ, and Headscale adapters.
// Revoke must undo the corresponding Apply operation.
type Backend interface {
	Apply(context.Context, Action) error
	Revoke(context.Context, Action) error
}

// Enforcer tracks deadlines independently of backend-specific rule storage. Its backend is
// always a SignedBackend, so every action is verified before it is installed (SHR-009).
// Pending actions live in memory only: I12 and I13 across a restart are not met (SHR-003).
type Enforcer struct {
	backend Backend
	mu      sync.Mutex
	actions map[string]pendingAction
	// spent holds revoked IDs until their signed deadline, so a replay cannot reinstall a
	// rule that was revoked at the capped deadline while its signature is still valid.
	spent map[string]time.Time
	now   func() time.Time
	logf  func(format string, args ...any)
}

type pendingAction struct {
	action    Action
	expiresAt time.Time
	// revoking marks an action whose revoke is in flight outside the lock.
	revoking bool
}

// NewEnforcer is the only way to build an Enforcer. It refuses a SignedBackend that could
// not verify a signature, so no composition installs an unsigned action.
func NewEnforcer(signed SignedBackend) (*Enforcer, error) {
	if err := signed.validate(); err != nil {
		return nil, err
	}
	return &Enforcer{
		backend: signed, actions: make(map[string]pendingAction), spent: make(map[string]time.Time),
		now: time.Now, logf: log.Printf,
	}, nil
}

func (e *Enforcer) Enforce(ctx context.Context, action Action) (Receipt, error) {
	if action.ID == "" || action.Target == "" || e.backend == nil || !validKind(action.Kind) {
		return Receipt{}, ErrInvalidAction
	}
	if err := ValidateTTL(action.TTL); err != nil {
		return Receipt{}, err
	}
	if action.Broad && !action.Approved {
		return Receipt{}, ErrApprovalRequired
	}
	now := e.now()
	expiresAt, err := deadline(action, now)
	if err != nil {
		return Receipt{}, err
	}
	// The signed deadline is CreatedAt+TTL; the tracked one is never later than now+TTL, so a
	// creation time inside the skew allowance cannot stretch a rule past MaxTTL (SHR-005).
	tracked := expiresAt
	if limit := now.Add(action.TTL); tracked.After(limit) {
		tracked = limit
	}
	// Receipt.ExpiresAt is the authoritative deadline: the enforcer revokes then, even when
	// the signed ExpiresAt the backend sees is later (by at most MaxClockSkew).
	receipt := Receipt{ActionID: action.ID, ExpiresAt: tracked}

	// Serialize the state transition with Apply so duplicate IDs cannot create
	// two backend rules. Backends should keep Apply bounded and idempotent.
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.actions[action.ID]; exists {
		return Receipt{}, ErrDuplicateAction
	}
	if until, revoked := e.spent[action.ID]; revoked && now.Before(until) {
		return Receipt{}, fmt.Errorf("%w: already enforced and revoked", ErrDuplicateAction)
	}
	if err := e.backend.Apply(ctx, action); err != nil {
		return Receipt{}, err
	}
	e.actions[action.ID] = pendingAction{action: action, expiresAt: receipt.ExpiresAt}
	return receipt, nil
}

// deadline checks the signed creation time against now and returns CreatedAt+TTL. It refuses
// a missing or future-dated creation time and an action whose deadline has passed (SHR-002).
func deadline(action Action, now time.Time) (time.Time, error) {
	if action.CreatedAt.IsZero() {
		return time.Time{}, fmt.Errorf("%w: creation time is required", ErrInvalidAction)
	}
	if action.CreatedAt.After(now.Add(MaxClockSkew)) {
		return time.Time{}, fmt.Errorf("%w: created in the future", ErrInvalidTTL)
	}
	expiresAt := action.CreatedAt.Add(action.TTL)
	if !action.ExpiresAt.IsZero() && !action.ExpiresAt.Equal(expiresAt) {
		return time.Time{}, ErrInvalidTTL
	}
	if !now.Before(expiresAt) {
		return time.Time{}, ErrActionExpired
	}
	return expiresAt, nil
}

// Expire revokes every action due at now. It tries each one even when an earlier revoke
// fails, so one stuck rule cannot hold back the others (SHR-004). Failed actions remain
// tracked so a later pass retries them rather than letting them become permanent. The
// returned error joins every failure; the slice lists only the actions actually revoked.
func (e *Enforcer) Expire(ctx context.Context, now time.Time) ([]string, error) {
	expired, _, err := e.expire(ctx, now)
	return expired, err
}

// expire is Expire that also names the IDs whose revoke failed. Revokes run outside the
// lock, so a slow backend never blocks Enforce; an in-flight action is skipped by a
// concurrent pass and cannot be revoked twice.
func (e *Enforcer) expire(ctx context.Context, now time.Time) (expired, failed []string, err error) {
	e.mu.Lock()
	due := make([]pendingAction, 0)
	for id, pending := range e.actions {
		if !pending.revoking && !now.Before(pending.expiresAt) {
			pending.revoking = true
			e.actions[id] = pending
			due = append(due, pending)
		}
	}
	for id, until := range e.spent {
		if !now.Before(until) {
			delete(e.spent, id)
		}
	}
	e.mu.Unlock()
	sort.Slice(due, func(i, j int) bool { return due[i].action.ID < due[j].action.ID })

	var failures []error
	for _, pending := range due {
		id := pending.action.ID
		revokeErr := e.backend.Revoke(ctx, pending.action)
		e.mu.Lock()
		if revokeErr != nil {
			pending.revoking = false
			e.actions[id] = pending
			failed = append(failed, id)
			failures = append(failures, fmt.Errorf("revoke %s: %w", id, revokeErr))
		} else {
			delete(e.actions, id)
			e.spent[id] = pending.action.CreatedAt.Add(pending.action.TTL)
			expired = append(expired, id)
		}
		e.mu.Unlock()
	}
	return expired, failed, errors.Join(failures...)
}

// Run reaps once immediately, then on every tick, until the context is canceled. A failed
// revoke is logged and retried on the next pass; it never stops the loop (SHR-004, I13).
func (e *Enforcer) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("expiration interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	reported := ""
	for {
		// Log when the set of failing IDs changes, not on every pass, and name IDs only:
		// a backend error can quote the target (go.md, Errors).
		_, failed, _ := e.expire(ctx, e.now())
		if current := strings.Join(failed, ","); current != reported {
			if current != "" {
				e.logf("enforcerd: revocation failed for %s; retrying every pass", current)
			} else {
				e.logf("enforcerd: pending revocations succeeded")
			}
			reported = current
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// validate reports whether s can verify a signature at all. It is checked once, at
// construction, so a misconfigured enforcer refuses to start rather than to install.
func (s SignedBackend) validate() error {
	switch {
	case s.Backend == nil:
		return errors.New("signed backend is not configured")
	case len(s.PublicKey) != ed25519.PublicKeySize:
		return errors.New("signed backend needs an Ed25519 public key")
	case !s.Breadth.valid():
		return errors.New("signed backend needs breadth thresholds")
	}
	switch s.MinVersion {
	case "", "v1", "v2":
		return nil
	default:
		return fmt.Errorf("signed backend: unknown MinVersion %q", s.MinVersion)
	}
}

func validKind(kind Kind) bool {
	return kind == KindNFTables || kind == KindRPZ || kind == KindHeadscale
}
