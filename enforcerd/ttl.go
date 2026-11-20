package enforcerd

import "time"

const MaxTTL = 24 * time.Hour

// ValidateTTL is shared by signing, approval, and enforcement boundaries.
func ValidateTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > MaxTTL {
		return ErrInvalidTTL
	}
	return nil
}
