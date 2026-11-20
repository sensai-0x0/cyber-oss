package enforcerd

import (
	"context"
	"crypto/ed25519"
	"errors"
	"sync"
	"testing"
	"time"
)

// testNow is the fixed clock the lifecycle tests share. Signed fixtures elsewhere use the same instant.
var testNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func fixedClock() time.Time { return testNow }

// signedEnforcer builds the standard composition: an Enforcer over a SignedBackend over a fake.
func signedEnforcer(t *testing.T, backend Backend) (*Enforcer, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewEnforcer(SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth, Now: fixedClock})
	if err != nil {
		t.Fatal(err)
	}
	e.now = fixedClock
	return e, private
}

func sign(t *testing.T, private ed25519.PrivateKey, action Action) Action {
	t.Helper()
	if err := SignAction(private, &action); err != nil {
		t.Fatal(err)
	}
	return action
}

func narrow(id string, created time.Time, ttl time.Duration) Action {
	return Action{ID: id, Kind: KindRPZ, Target: "phish.example", TTL: ttl, Approved: true, CreatedAt: created}
}

// SHR-009: the constructor refuses any composition that cannot verify a signature.
func TestNewEnforcerRequiresAVerifyingBackend(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(nil)
	cases := map[string]SignedBackend{
		"zero value":     {},
		"no backend":     {PublicKey: public, Breadth: testBreadth},
		"no key":         {Backend: &fakeBackend{}, Breadth: testBreadth},
		"short key":      {PublicKey: public[:8], Backend: &fakeBackend{}, Breadth: testBreadth},
		"no thresholds":  {PublicKey: public, Backend: &fakeBackend{}},
		"bad MinVersion": {PublicKey: public, Backend: &fakeBackend{}, Breadth: testBreadth, MinVersion: "V2"},
	}
	for name, signed := range cases {
		if e, err := NewEnforcer(signed); err == nil || e != nil {
			t.Errorf("%s: NewEnforcer = %v, %v; want an error", name, e, err)
		}
	}
}

// SHR-009: the standard path rejects an unsigned action before any backend call.
func TestUnsignedActionIsRejectedByTheStandardPath(t *testing.T) {
	backend := &fakeBackend{}
	e, _ := signedEnforcer(t, backend)
	if _, err := e.Enforce(context.Background(), narrow("unsigned", testNow, time.Minute)); err == nil {
		t.Fatal("unsigned action enforced")
	}
	if backend.applyCalls() != 0 {
		t.Fatal("unsigned action reached the backend")
	}
}

// SHR-002: a correctly signed action is refused once its deadline has passed (-1s, 0, +1s).
func TestSignedBackendRejectsExpiredActions(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	action := sign(t, private, narrow("deadline", testNow, time.Minute))
	deadline := testNow.Add(time.Minute)
	for _, tc := range []struct {
		offset time.Duration
		ok     bool
	}{{-time.Second, true}, {0, false}, {time.Second, false}} {
		at := deadline.Add(tc.offset)
		backend := &recordingBackend{}
		signed := SignedBackend{PublicKey: public, Backend: backend, Breadth: testBreadth, Now: func() time.Time { return at }}
		err := signed.Apply(context.Background(), action)
		if tc.ok && err != nil {
			t.Errorf("deadline%+v: refused a live action: %v", tc.offset, err)
		}
		if !tc.ok && (!errors.Is(err, ErrActionExpired) || backend.applied != 0) {
			t.Errorf("deadline%+v: err = %v, applied = %d; want ErrActionExpired and no apply", tc.offset, err, backend.applied)
		}
	}
}

// SHR-002/SHR-005: an action dated beyond the clock-skew allowance is refused.
func TestSignedBackendRejectsFutureDatedActions(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(nil)
	signed := SignedBackend{PublicKey: public, Backend: &recordingBackend{}, Breadth: testBreadth, Now: fixedClock}
	edge := sign(t, private, narrow("edge", testNow.Add(MaxClockSkew), time.Minute))
	if err := signed.Apply(context.Background(), edge); err != nil {
		t.Fatalf("refused an action dated at the skew limit: %v", err)
	}
	late := sign(t, private, narrow("late", testNow.Add(MaxClockSkew+time.Second), time.Minute))
	if err := signed.Apply(context.Background(), late); !errors.Is(err, ErrInvalidTTL) {
		t.Fatalf("future-dated err = %v, want ErrInvalidTTL", err)
	}
}

// SHR-005: CreatedAt 23h ahead with a 24h TTL must not yield a 47h deadline. The skew check
// refuses it outright; within the allowance the tracked deadline is still capped at now+TTL.
func TestEnforcerDeadlineNeverExceedsNowPlusTTL(t *testing.T) {
	backend := &fakeBackend{}
	e, private := signedEnforcer(t, backend)
	far := sign(t, private, narrow("far", testNow.Add(23*time.Hour), MaxTTL))
	if _, err := e.Enforce(context.Background(), far); err == nil {
		t.Fatal("enforced an action dated 23h ahead")
	}
	near := sign(t, private, narrow("near", testNow.Add(MaxClockSkew), MaxTTL))
	receipt, err := e.Enforce(context.Background(), near)
	if err != nil {
		t.Fatal(err)
	}
	if limit := testNow.Add(MaxTTL); receipt.ExpiresAt.After(limit) {
		t.Fatalf("deadline %v is past now+TTL %v", receipt.ExpiresAt, limit)
	}
	past := sign(t, private, narrow("past", testNow.Add(-30*time.Minute), time.Hour))
	receipt, err = e.Enforce(context.Background(), past)
	if err != nil {
		t.Fatal(err)
	}
	if want := testNow.Add(30 * time.Minute); !receipt.ExpiresAt.Equal(want) {
		t.Fatalf("past-dated deadline = %v, want %v", receipt.ExpiresAt, want)
	}
}

// A replayed action is refused while it is tracked, and refused as expired once it is not.
func TestReplayIsRefusedBeforeAndAfterExpiry(t *testing.T) {
	backend := &fakeBackend{}
	e, private := signedEnforcer(t, backend)
	action := sign(t, private, narrow("once", testNow, time.Minute))
	if _, err := e.Enforce(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Enforce(context.Background(), action); !errors.Is(err, ErrDuplicateAction) {
		t.Fatalf("replay while tracked err = %v", err)
	}
	after := testNow.Add(time.Minute)
	if _, err := e.Expire(context.Background(), after); err != nil {
		t.Fatal(err)
	}
	e.now = func() time.Time { return after }
	e.backend = SignedBackend{PublicKey: e.backend.(SignedBackend).PublicKey, Backend: backend, Breadth: testBreadth, Now: e.now}
	if _, err := e.Enforce(context.Background(), action); !errors.Is(err, ErrActionExpired) {
		t.Fatalf("replay after expiry err = %v, want ErrActionExpired", err)
	}
	if backend.applyCalls() != 1 {
		t.Fatalf("applied %d times, want 1", backend.applyCalls())
	}
}

// flakyRevoker fails the first `failures` revokes of each id and reports every call.
type flakyRevoker struct {
	fakeBackend
	mu       sync.Mutex
	failures int
	tries    map[string]int
	calls    chan string
}

func (f *flakyRevoker) Revoke(ctx context.Context, action Action) error {
	f.mu.Lock()
	f.tries[action.ID]++
	failed := f.tries[action.ID] <= f.failures
	f.mu.Unlock()
	if f.calls != nil {
		f.calls <- action.ID
	}
	if failed {
		return errors.New("backend unavailable")
	}
	return f.fakeBackend.Revoke(ctx, action)
}

// SHR-004: one failing revoke does not stop the others, and the failure stays tracked.
func TestExpireRevokesEveryDueActionPastAFailure(t *testing.T) {
	backend := &flakyRevoker{failures: 1, tries: map[string]int{}}
	e, private := signedEnforcer(t, backend)
	for _, id := range []string{"a", "b"} {
		if _, err := e.Enforce(context.Background(), sign(t, private, narrow(id, testNow, time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	backend.tries["b"] = 1 // only "a" fails
	expired, err := e.Expire(context.Background(), testNow.Add(time.Minute))
	if err == nil {
		t.Fatal("a failed revoke was not reported")
	}
	if len(expired) != 1 || expired[0] != "b" {
		t.Fatalf("expired = %v, want [b]", expired)
	}
	expired, err = e.Expire(context.Background(), testNow.Add(time.Minute))
	if err != nil || len(expired) != 1 || expired[0] != "a" {
		t.Fatalf("retry: expired = %v, err = %v; want [a]", expired, err)
	}
}

// SHR-004: Run keeps going after a failed revoke and retries it on a later tick.
func TestRunRetriesAFailedRevokeOnALaterTick(t *testing.T) {
	backend := &flakyRevoker{failures: 1, tries: map[string]int{}, calls: make(chan string, 4)}
	e, private := signedEnforcer(t, backend)
	if _, err := e.Enforce(context.Background(), sign(t, private, narrow("r", testNow, time.Minute))); err != nil {
		t.Fatal(err)
	}
	e.now = func() time.Time { return testNow.Add(time.Minute) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx, time.Millisecond) }()
	for range 2 {
		select {
		case <-backend.calls:
		case err := <-done:
			t.Fatalf("Run stopped after a failed revoke: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("revoke was not retried")
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
	if backend.revokeCalls() != 1 {
		t.Fatalf("successful revokes = %d, want 1", backend.revokeCalls())
	}
}

// I13: Run reaps once on start, before the first tick.
func TestRunReapsOnStartBeforeTheFirstTick(t *testing.T) {
	backend := &flakyRevoker{tries: map[string]int{}, calls: make(chan string, 1)}
	e, private := signedEnforcer(t, backend)
	if _, err := e.Enforce(context.Background(), sign(t, private, narrow("due", testNow, time.Minute))); err != nil {
		t.Fatal(err)
	}
	e.now = func() time.Time { return testNow.Add(time.Minute) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.Run(ctx, time.Hour) }()
	select {
	case id := <-backend.calls:
		if id != "due" {
			t.Fatalf("revoked %q", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no reap before the first tick")
	}
}
