// Package operator implements the approval side of the mitigation boundary.
package operator

import (
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/metacogna/sentinel-hunter/enforcerd"
)

func Approve(action enforcerd.Action, privateKey ed25519.PrivateKey, name string, ttl time.Duration, now time.Time, breadth enforcerd.Breadth) (enforcerd.Action, error) {
	if name == "" {
		return enforcerd.Action{}, errors.New("operator name is required")
	}
	if ttl <= 0 || ttl > enforcerd.MaxTTL {
		return enforcerd.Action{}, enforcerd.ErrInvalidTTL
	}
	// Broad is derived from the target; a claimed flag that disagrees is refused, never signed (SHR-001).
	if err := breadth.Check(action.Target, action.Broad); err != nil {
		return enforcerd.Action{}, err
	}
	action.TTL = ttl
	action.Approved = true
	action.ApprovedBy = name
	action.CreatedAt = now.UTC()
	action.ExpiresAt = action.CreatedAt.Add(ttl)
	if err := enforcerd.SignAction(privateKey, &action); err != nil {
		return enforcerd.Action{}, err
	}
	return action, nil
}

func Reject(action enforcerd.Action, _ ed25519.PrivateKey, name string, now time.Time) (enforcerd.Action, error) {
	if name == "" {
		return enforcerd.Action{}, errors.New("operator name is required")
	}
	action.Approved = false
	action.ApprovedBy = name
	action.CreatedAt = now.UTC()
	action.ExpiresAt = time.Time{}
	action.Signature = ""
	return action, nil
}
