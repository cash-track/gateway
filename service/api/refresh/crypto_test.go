package refresh

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cash-track/gateway/headers/cookie"
)

func TestKey(t *testing.T) {
	k1 := Key("refresh-token-a")
	k2 := Key("refresh-token-b")

	assert.Len(t, k1, 64) // hex-encoded sha256: 32 bytes -> 64 hex chars
	assert.NotEqual(t, k1, k2)
	assert.Equal(t, k1, Key("refresh-token-a")) // deterministic
	assert.NotContains(t, k1, "refresh-token-a") // never the raw token
}

func TestEncryptDecryptAuthRoundTrip(t *testing.T) {
	key := Key("some-refresh-token")
	auth := cookie.Auth{
		AccessToken:           "access",
		AccessTokenExpiredAt:  "2026-01-01T00:00:00Z",
		RefreshToken:          "refresh",
		RefreshTokenExpiredAt: "2026-02-01T00:00:00Z",
	}

	payload, err := encryptAuth(key, auth)
	assert.NoError(t, err)
	assert.NotEmpty(t, payload)
	assert.NotContains(t, payload, "refresh") // ciphertext, not plaintext

	got, err := decryptAuth(key, payload)
	assert.NoError(t, err)
	assert.Equal(t, auth, got)
}

func TestEncryptAuthInvalidKey(t *testing.T) {
	_, err := encryptAuth("not-hex-at-all-zz", cookie.Auth{})
	assert.Error(t, err)
}

func TestEncryptAuthKeyWrongLength(t *testing.T) {
	shortKey := hex.EncodeToString([]byte("too-short"))

	_, err := encryptAuth(shortKey, cookie.Auth{})
	assert.Error(t, err)
}

func TestDecryptAuthInvalidKey(t *testing.T) {
	_, err := decryptAuth("not-hex-at-all-zz", "irrelevant")
	assert.Error(t, err)
}

func TestDecryptAuthInvalidBase64(t *testing.T) {
	key := Key("token")

	_, err := decryptAuth(key, "not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecryptAuthPayloadShorterThanNonce(t *testing.T) {
	key := Key("token")
	tooShort := base64.StdEncoding.EncodeToString([]byte("x"))

	_, err := decryptAuth(key, tooShort)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shorter than nonce size")
}

func TestDecryptAuthTamperedCiphertextFailsAuthentication(t *testing.T) {
	key := Key("token")
	payload, err := encryptAuth(key, cookie.Auth{AccessToken: "access"})
	assert.NoError(t, err)

	raw, err := base64.StdEncoding.DecodeString(payload)
	assert.NoError(t, err)
	raw[len(raw)-1] ^= 0xFF // flip the last ciphertext byte
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = decryptAuth(key, tampered)
	assert.Error(t, err)
}

func TestDecryptAuthWrongKeyFailsToDecrypt(t *testing.T) {
	encKey := Key("token-a")
	decKey := Key("token-b")

	payload, err := encryptAuth(encKey, cookie.Auth{AccessToken: "access"})
	assert.NoError(t, err)

	_, err = decryptAuth(decKey, payload)
	assert.Error(t, err)
}

// TestDecryptAuthCorruptJSONAfterSuccessfulDecryption crafts an authenticated GCM
// payload whose plaintext is not valid JSON, to exercise the json.Unmarshal error
// branch in decryptAuth — unreachable through the public encryptAuth path since it
// always marshals a valid cookie.Auth first.
func TestDecryptAuthCorruptJSONAfterSuccessfulDecryption(t *testing.T) {
	key := Key("token")

	aesk, err := aesKey(key)
	assert.NoError(t, err)

	block, err := aes.NewCipher(aesk)
	assert.NoError(t, err)

	gcm, err := cipher.NewGCM(block)
	assert.NoError(t, err)

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	assert.NoError(t, err)

	ciphertext := gcm.Seal(nonce, nonce, []byte("not json"), nil)
	payload := base64.StdEncoding.EncodeToString(ciphertext)

	_, err = decryptAuth(key, payload)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal auth")
}

func TestEncryptAuthNonceGenerationFailure(t *testing.T) {
	orig := randReader
	t.Cleanup(func() { randReader = orig })
	randReader = failingReader{}

	_, err := encryptAuth(Key("token"), cookie.Auth{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generate nonce")
}

// failingReader always errors, for forcing randReader's failure path in tests.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func TestAesKeyInvalidHex(t *testing.T) {
	_, err := aesKey("zz-not-hex")
	assert.Error(t, err)
}

func TestAesKeyWrongByteLength(t *testing.T) {
	// 16 bytes decodes fine but isn't the required 32-byte AES-256 key size.
	_, err := aesKey(hex.EncodeToString(make([]byte, 16)))
	assert.Error(t, err)
}

func TestAesKeyValid(t *testing.T) {
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	assert.NoError(t, err)

	got, err := aesKey(hex.EncodeToString(raw))
	assert.NoError(t, err)
	assert.Equal(t, raw, got)
}
