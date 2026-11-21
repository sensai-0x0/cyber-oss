package operator

import (
	"crypto/ed25519"
	"errors"

	"github.com/metacogna/sentinel-hunter/hunterd/scope"
)

// SignScope signs a scope bundle (fleet plan Phase 1). hunterd/scope only verifies (Parse);
// the actual ed25519.Sign call lives here, on the approval side of the trust boundary, per
// hunterd/CLAUDE.md ("No mitigation, firewall or signing code here").
func SignScope(privateKey ed25519.PrivateKey, keyID string, b scope.Bundle) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("operator: invalid Ed25519 private key")
	}
	canonical, err := scope.CanonicalBytes(keyID, b)
	if err != nil {
		return nil, err
	}
	return scope.Assemble(keyID, b, ed25519.Sign(privateKey, canonical))
}
