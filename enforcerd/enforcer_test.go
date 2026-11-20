package enforcerd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBroadActionRequiresApproval(t *testing.T) {
	backend := &fakeBackend{}
	e, private := signedEnforcer(t, backend)
	_, err := e.Enforce(context.Background(), sign(t, private, Action{
		ID: "broad", Kind: KindNFTables, Target: "192.0.2.0/24", TTL: time.Minute, Broad: true, CreatedAt: testNow,
	}))
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("error = %v, want ErrApprovalRequired", err)
	}
	if backend.applyCalls() != 0 {
		t.Fatal("unapproved broad action reached backend")
	}
}

func TestActionIsTTLBoundedAndExpires(t *testing.T) {
	backend := &fakeBackend{}
	e, private := signedEnforcer(t, backend)
	now := testNow
	action := sign(t, private, narrow("temporary", now, time.Minute))
	if _, err := e.Enforce(context.Background(), action); err != nil {
		t.Fatal(err)
	}

	expired, err := e.Expire(context.Background(), now.Add(59*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 0 || backend.revokeCalls() != 0 {
		t.Fatal("action expired before its deadline")
	}
	expired, err = e.Expire(context.Background(), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0] != "temporary" || backend.revokeCalls() != 1 {
		t.Fatalf("expiration = %v, revocations = %d", expired, backend.revokeCalls())
	}
}

func TestRejectsMissingTTLAndDuplicateID(t *testing.T) {
	backend := &fakeBackend{}
	e, private := signedEnforcer(t, backend)
	if _, err := e.Enforce(context.Background(), Action{ID: "no-ttl", Kind: KindRPZ, Target: "a.example", CreatedAt: testNow}); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("missing TTL error = %v", err)
	}
	action := sign(t, private, narrow("same", testNow, time.Minute))
	if _, err := e.Enforce(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Enforce(context.Background(), action); !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("duplicate error = %v", err)
	}
}

type fakeBackend struct {
	mu      sync.Mutex
	applied []Action
	revoked []string
}

func (f *fakeBackend) Apply(_ context.Context, action Action) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, action)
	return nil
}

func (f *fakeBackend) Revoke(_ context.Context, action Action) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked = append(f.revoked, action.ID)
	return nil
}

func (f *fakeBackend) applyCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.applied)
}

func (f *fakeBackend) revokeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.revoked)
}
