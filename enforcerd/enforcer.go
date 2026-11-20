// Package enforcerd enforces approved, reversible actions with mandatory TTLs.
package enforcerd

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrApprovalRequired = errors.New("operator approval required for broad action")
	ErrInvalidTTL       = errors.New("TTL must be greater than zero and no more than 24 hours")
	ErrDuplicateAction  = errors.New("mitigation action already exists")
	ErrInvalidAction    = errors.New("invalid mitigation action")
)

type Kind string

const (
	KindNFTables  Kind = "nftables"
	KindRPZ       Kind = "rpz"
	KindHeadscale Kind = "headscale"
)

// Action is backend-neutral; Broad actions must be explicitly approved.
type Action struct {
	ID         string
	Kind       Kind
	Target     string
	TTL        time.Duration
	Broad      bool
	Approved   bool
	ApprovedBy string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Signature  string
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

// Enforcer tracks deadlines independently of backend-specific rule storage.
type Enforcer struct {
	backend Backend
	mu      sync.Mutex
	actions map[string]pendingAction
	now     func() time.Time
}

type pendingAction struct {
	action    Action
	expiresAt time.Time
}

func NewEnforcer(backend Backend) *Enforcer {
	return &Enforcer{backend: backend, actions: make(map[string]pendingAction), now: time.Now}
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
	if action.CreatedAt.IsZero() {
		action.CreatedAt = e.now().UTC()
	}
	expiresAt := action.CreatedAt.Add(action.TTL)
	if !action.ExpiresAt.IsZero() && !action.ExpiresAt.Equal(expiresAt) {
		return Receipt{}, ErrInvalidTTL
	}
	action.ExpiresAt = expiresAt
	receipt := Receipt{ActionID: action.ID, ExpiresAt: expiresAt}

	// Serialize the state transition with Apply so duplicate IDs cannot create
	// two backend rules. Backends should keep Apply bounded and idempotent.
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.actions[action.ID]; exists {
		return Receipt{}, ErrDuplicateAction
	}
	if err := e.backend.Apply(ctx, action); err != nil {
		return Receipt{}, err
	}
	e.actions[action.ID] = pendingAction{action: action, expiresAt: receipt.ExpiresAt}
	return receipt, nil
}

// Expire revokes every action due at now. Failed actions remain tracked so a
// later pass can retry revocation rather than silently becoming permanent.
func (e *Enforcer) Expire(ctx context.Context, now time.Time) ([]string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := make([]string, 0)
	for id, pending := range e.actions {
		if !now.Before(pending.expiresAt) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	expired := make([]string, 0, len(ids))
	for _, id := range ids {
		pending := e.actions[id]
		if err := e.backend.Revoke(ctx, pending.action); err != nil {
			return expired, err
		}
		delete(e.actions, id)
		expired = append(expired, id)
	}
	return expired, nil
}

// Run expires actions until the context is canceled.
func (e *Enforcer) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("expiration interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if _, err := e.Expire(ctx, now); err != nil {
				return err
			}
		}
	}
}

func validKind(kind Kind) bool {
	return kind == KindNFTables || kind == KindRPZ || kind == KindHeadscale
}
