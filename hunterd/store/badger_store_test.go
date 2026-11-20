package store

import (
	"crypto/sha256"
	"testing"
	"time"
)

func TestDeltaGateNovelUnchangedMutated(t *testing.T) {
	store := openTestStore(t)
	gate := NewDeltaGate(store, time.Hour)
	key := "tls/192.0.2.10:443"
	first := sha256.Sum256([]byte("certificate-a"))
	second := sha256.Sum256([]byte("certificate-b"))
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	decision, err := gate.Apply(key, first, []byte(`{"jarm":"a"}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != Novel || !decision.Emit {
		t.Fatalf("first observation = %+v, want emitted novel event", decision)
	}

	decision, err = gate.Apply(key, first, []byte(`{"ignored":true}`), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != Unchanged || decision.Emit {
		t.Fatalf("repeated observation = %+v, want suppressed unchanged event", decision)
	}

	decision, err = gate.Apply(key, second, []byte(`{"jarm":"b"}`), now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != Mutated || !decision.Emit {
		t.Fatalf("changed observation = %+v, want emitted mutated event", decision)
	}
	if string(decision.Payload) != `{"jarm":"b"}` {
		t.Fatalf("payload = %s", decision.Payload)
	}
}

func TestCheckpointRoundTrip(t *testing.T) {
	store := openTestStore(t)
	if err := store.SaveCursor("authorized-scope-a", 42); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadCursor("authorized-scope-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("cursor = %d, want 42", got)
	}
}

func TestObservationPayloadIsCopied(t *testing.T) {
	store := openTestStore(t)
	gate := NewDeltaGate(store, time.Hour)
	payload := []byte("original")
	hash := sha256.Sum256(payload)
	decision, err := gate.Apply("asset", hash, payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = 'X'
	if string(decision.Payload) != "original" {
		t.Fatal("returned payload aliases caller memory")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}
