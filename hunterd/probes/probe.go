// Package probes converts tlsx and httpx results into the shared delta-event
// contract. Active methods must only be used against authorized targets.
package probes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Observation is the v1 Go representation of schemas/delta_event.v1.json.
type Observation struct {
	SchemaVersion string         `json:"schema_version"`
	EventID       string         `json:"event_id"`
	Kind          string         `json:"kind"`
	Source        string         `json:"source"`
	Target        string         `json:"target"`
	ObservedAt    time.Time      `json:"observed_at"`
	Fingerprint   string         `json:"fingerprint"`
	Attributes    map[string]any `json:"attributes"`
}

func newObservation(source, target string, observedAt time.Time, attributes map[string]any) Observation {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	observedAt = observedAt.UTC()
	canonical, _ := json.Marshal(struct {
		Source     string         `json:"source"`
		Target     string         `json:"target"`
		ObservedAt time.Time      `json:"observed_at"`
		Attributes map[string]any `json:"attributes"`
	}{source, target, observedAt, attributes})
	digest := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(digest[:])
	return Observation{
		SchemaVersion: "v1",
		EventID:       fingerprint[:32],
		Kind:          "novel",
		Source:        source,
		Target:        target,
		ObservedAt:    observedAt,
		Fingerprint:   fingerprint,
		Attributes:    attributes,
	}
}
