package jwks

import "crypto/rsa"

// Provider resolves the API's current RSA signing keys by kid, holding them in memory.
type Provider interface {
	// Key returns the RSA public key for kid, triggering a rate-limited refresh first if
	// kid is unknown, so an API-side key rotation heals without a gateway restart.
	Key(kid string) (*rsa.PublicKey, bool)

	// Loaded reports whether any key material is held. False means callers should fail
	// open rather than reject every token.
	Loaded() bool
}
