package enforcerd

import "time"

const MaxTTL = 24 * time.Hour

// MaxClockSkew is how far ahead of the enforcer's clock a signed creation time may be.
// A later CreatedAt would let a caller push a rule's deadline past now+MaxTTL (SHR-005).
const MaxClockSkew = time.Minute

// ValidateTTL is shared by signing, approval, and enforcement boundaries.
func ValidateTTL(ttl time.Duration) error {
	if ttl <= 0 || ttl > MaxTTL {
		return ErrInvalidTTL
	}
	return nil
}
