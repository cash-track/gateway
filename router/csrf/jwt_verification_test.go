package csrf

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

// stubJwksProvider is a hand-written test double for jwks.Provider - the interface is
// tiny enough that a generated mock would only add ceremony.
type stubJwksProvider struct {
	loaded bool
	keys   map[string]*rsa.PublicKey
}

func (s *stubJwksProvider) Key(kid string) (*rsa.PublicKey, bool) {
	key, ok := s.keys[kid]

	return key, ok
}

func (s *stubJwksProvider) Loaded() bool {
	return s.loaded
}

// unloadedJwksProvider mirrors the state before any JWKS fetch ever succeeded (or an
// HS256-configured API returning an empty key set): CSRF must fail open to the legacy
// unverified decode.
func unloadedJwksProvider() *stubJwksProvider {
	return &stubJwksProvider{loaded: false}
}

func loadedJwksProvider(keys map[string]*rsa.PublicKey) *stubJwksProvider {
	return &stubJwksProvider{loaded: true, keys: keys}
}

func generateTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	return key
}

func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		token.Header["kid"] = kid
	}

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return signed
}

func TestGetUserContextFromAccessTokenRS256Verified(t *testing.T) {
	key := generateTestRSAKey(t)
	kid := "abcdef0123456789"

	for name, test := range map[string]struct {
		provider      *stubJwksProvider
		token         func() string
		expectContext string
		expectError   bool
	}{
		"ValidSignatureAccepted": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				return signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})
			},
			expectContext: "123987:987654321",
		},
		"ExpiredButGenuineSignatureStillYieldsValidContext": {
			// Load-bearing: claims validation must stay OFF. The proxy relies on an
			// expired-but-genuine token still reaching it so a 401 can trigger refresh.
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				return signRS256(t, key, kid, jwt.MapClaims{
					"sub": 123987,
					"iat": 987654321,
					"exp": time.Now().Add(-24 * time.Hour).Unix(),
				})
			},
			expectContext: "123987:987654321",
		},
		"TamperedSignatureRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				signed := signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})
				parts := strings.Split(signed, ".")

				raw, err := base64.RawURLEncoding.DecodeString(parts[2])
				if err != nil {
					t.Fatalf("failed to decode signature: %v", err)
				}
				// Flip a byte in the middle of the signature - unlike its low-order
				// trailing bits, this is guaranteed to change the decoded value.
				raw[len(raw)/2] ^= 0xFF
				parts[2] = base64.RawURLEncoding.EncodeToString(raw)

				return strings.Join(parts, ".")
			},
			expectError: true,
		},
		"SignedByDifferentKeyRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &generateTestRSAKey(t).PublicKey}),
			token: func() string {
				return signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})
			},
			expectError: true,
		},
		"AlgNoneRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": 123987, "iat": 987654321})
				token.Header["kid"] = kid
				signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
				if err != nil {
					t.Fatalf("failed to sign none token: %v", err)
				}

				return signed
			},
			expectError: true,
		},
		"HS256Rejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": 123987, "iat": 987654321}).SignedString([]byte("asd"))
				if err != nil {
					t.Fatalf("failed to sign HS256 token: %v", err)
				}

				return s
			},
			expectError: true,
		},
		"UnknownKidRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{"other-kid": &key.PublicKey}),
			token: func() string {
				return signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})
			},
			expectError: true,
		},
		"MissingKidHeaderRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				return signRS256(t, key, "", jwt.MapClaims{"sub": 123987, "iat": 987654321})
			},
			expectError: true,
		},
		"MalformedTokenRejected": {
			provider: loadedJwksProvider(map[string]*rsa.PublicKey{kid: &key.PublicKey}),
			token: func() string {
				return "not.a.jwt"
			},
			expectError: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			handler := NewRedisHandler(nil, test.provider)

			ctx, err := handler.getUserContextFromAccessToken(test.token())

			if test.expectError {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectContext, ctx)
		})
	}
}

func TestGetUserContextFromAccessTokenFailsOpenWithoutKeys(t *testing.T) {
	key := generateTestRSAKey(t)
	kid := "abcdef0123456789"

	// Even a genuinely RS256-signed token goes through the legacy unverified path when
	// no JWKS key material is loaded - this is the documented fail-open behaviour for an
	// HS256-configured API / local dev / a not-yet-completed initial fetch.
	token := signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})

	handler := NewRedisHandler(nil, unloadedJwksProvider())

	ctx, err := handler.getUserContextFromAccessToken(token)

	assert.NoError(t, err)
	assert.Equal(t, "123987:987654321", ctx)
}

func TestGetUserContextFromAccessTokenFailsOpenWhenProviderNil(t *testing.T) {
	key := generateTestRSAKey(t)
	token := signRS256(t, key, "kid", jwt.MapClaims{"sub": 123987, "iat": 987654321})

	handler := NewRedisHandler(nil, nil)

	ctx, err := handler.getUserContextFromAccessToken(token)

	assert.NoError(t, err)
	assert.Equal(t, "123987:987654321", ctx)
}

func TestKeyfuncUnknownKidTriggersRefreshOnProvider(t *testing.T) {
	// The refresh-on-unknown-kid behaviour itself lives in jwks.HttpProvider.Key and is
	// covered there; this only proves the CSRF Keyfunc calls Key exactly once and
	// surfaces its result either way.
	key := generateTestRSAKey(t)
	kid := "abcdef0123456789"

	calls := 0
	provider := &countingJwksProvider{
		loaded: true,
		key: func(k string) (*rsa.PublicKey, bool) {
			calls++

			return &key.PublicKey, k == kid
		},
	}

	handler := NewRedisHandler(nil, provider)
	token := signRS256(t, key, kid, jwt.MapClaims{"sub": 123987, "iat": 987654321})

	ctx, err := handler.getUserContextFromAccessToken(token)

	assert.NoError(t, err)
	assert.Equal(t, "123987:987654321", ctx)
	assert.Equal(t, 1, calls)
}

// countingJwksProvider lets a test observe how the CSRF Keyfunc calls Provider.Key.
type countingJwksProvider struct {
	loaded bool
	key    func(kid string) (*rsa.PublicKey, bool)
}

func (c *countingJwksProvider) Key(kid string) (*rsa.PublicKey, bool) {
	return c.key(kid)
}

func (c *countingJwksProvider) Loaded() bool {
	return c.loaded
}
