// Package store persists probe observations and scan checkpoints in BadgerDB.
package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// Store owns the embedded BadgerDB lifecycle and resumable scan cursors.
type Store struct{ db *badger.DB }

func Open(path string) (*Store, error) {
	db, err := badger.Open(badger.DefaultOptions(path).WithLogger(nil))
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) SaveCursor(scope string, cursor uint64) error {
	if scope == "" {
		return errors.New("checkpoint scope is required")
	}
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], cursor)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("checkpoint/"+scope), raw[:])
	})
}

func (s *Store) LoadCursor(scope string) (uint64, error) {
	if scope == "" {
		return 0, errors.New("checkpoint scope is required")
	}
	var cursor uint64
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("checkpoint/" + scope))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(raw []byte) error {
			if len(raw) != 8 {
				return errors.New("invalid checkpoint encoding")
			}
			cursor = binary.BigEndian.Uint64(raw)
			return nil
		})
	})
	return cursor, err
}

type DeltaKind string

const (
	Novel     DeltaKind = "novel"
	Unchanged DeltaKind = "unchanged"
	Mutated   DeltaKind = "mutated"
)

// Decision tells the producer whether an observation may cross the event
// channel. Payload is present for novel and mutated decisions only.
type Decision struct {
	Kind    DeltaKind
	Emit    bool
	Payload []byte
}

type observation struct {
	Version    int       `json:"version"`
	Hash       string    `json:"sha256"`
	Payload    []byte    `json:"payload"`
	ObservedAt time.Time `json:"observed_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// DeltaGate atomically suppresses unchanged observations and expires stale
// observation state using Badger's TTL support.
type DeltaGate struct {
	store *Store
	ttl   time.Duration
}

func NewDeltaGate(store *Store, ttl time.Duration) *DeltaGate {
	return &DeltaGate{store: store, ttl: ttl}
}

func (g *DeltaGate) Apply(key string, hash [sha256.Size]byte, payload []byte, observedAt time.Time) (Decision, error) {
	if key == "" {
		return Decision{}, errors.New("observation key is required")
	}
	if g.ttl <= 0 {
		return Decision{}, errors.New("observation TTL must be positive")
	}
	if observedAt.IsZero() {
		return Decision{}, errors.New("observation time is required")
	}
	payloadCopy := append([]byte(nil), payload...)
	decision := Decision{Payload: payloadCopy}
	err := g.store.db.Update(func(txn *badger.Txn) error {
		storageKey := []byte("observation/" + key)
		current := observation{}
		item, err := txn.Get(storageKey)
		switch {
		case errors.Is(err, badger.ErrKeyNotFound):
			decision.Kind, decision.Emit = Novel, true
			current = observation{Version: 1, Hash: hex.EncodeToString(hash[:]), Payload: payloadCopy, ObservedAt: observedAt, LastSeen: observedAt}
		case err != nil:
			return err
		default:
			if err := item.Value(func(raw []byte) error { return json.Unmarshal(raw, &current) }); err != nil {
				return fmt.Errorf("decode observation: %w", err)
			}
			if current.Hash == hex.EncodeToString(hash[:]) {
				decision.Kind, decision.Emit, decision.Payload = Unchanged, false, nil
				current.LastSeen = observedAt
			} else {
				decision.Kind, decision.Emit = Mutated, true
				current = observation{Version: 1, Hash: hex.EncodeToString(hash[:]), Payload: payloadCopy, ObservedAt: observedAt, LastSeen: observedAt}
			}
		}
		raw, err := json.Marshal(current)
		if err != nil {
			return err
		}
		return txn.SetEntry(badger.NewEntry(storageKey, raw).WithTTL(g.ttl))
	})
	if err != nil {
		return Decision{}, fmt.Errorf("apply delta gate: %w", err)
	}
	return decision, nil
}
