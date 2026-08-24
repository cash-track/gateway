package refresh

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cash-track/gateway/headers/cookie"
)

// Key derives the coordinator/lock/result key for a refresh token. It is a one-way
// hash: never log or store the raw refresh token itself as a key.
func Key(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))

	return hex.EncodeToString(sum[:])
}

// aesKeySize is 32 bytes: the required AES-256 key length.
const aesKeySize = 32

// randReader is a seam over crypto/rand.Reader so tests can force nonce generation to
// fail.
var randReader io.Reader = rand.Reader

// aesKey recovers the key material from a coordinator key (see Key). Reusing the hash
// as key material is safe: only holders of the original refresh token can derive it.
func aesKey(key string) ([]byte, error) {
	raw, err := hex.DecodeString(key)
	if err != nil {
		return nil, fmt.Errorf("decode coordinator key: %w", err)
	}

	if len(raw) != aesKeySize {
		return nil, fmt.Errorf("coordinator key must decode to %d bytes, got %d", aesKeySize, len(raw))
	}

	return raw, nil
}

// encryptAuth encrypts auth with AES-256-GCM under a key derived from key, for
// short-lived storage in Redis as the published refresh result.
func encryptAuth(key string, auth cookie.Auth) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}

	// cookie.Auth is a flat struct of strings — json.Marshal has no channel, func, or
	// cycle to choke on, so it structurally cannot fail here.
	plaintext, _ := json.Marshal(auth)

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(randReader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAuth reverses encryptAuth.
func decryptAuth(key, payload string) (cookie.Auth, error) {
	var auth cookie.Auth

	gcm, err := newGCM(key)
	if err != nil {
		return auth, err
	}

	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return auth, fmt.Errorf("decode payload: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return auth, fmt.Errorf("payload shorter than nonce size")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return auth, fmt.Errorf("decrypt payload: %w", err)
	}

	if err := json.Unmarshal(plaintext, &auth); err != nil {
		return auth, fmt.Errorf("unmarshal auth: %w", err)
	}

	return auth, nil
}

func newGCM(key string) (cipher.AEAD, error) {
	aesk, err := aesKey(key)
	if err != nil {
		return nil, err
	}

	// aesKey guarantees a 32-byte key — aes.NewCipher's only failure mode — and AES's
	// block size is always GCM-compatible, so neither call below can fail.
	block, _ := aes.NewCipher(aesk)
	gcm, _ := cipher.NewGCM(block)

	return gcm, nil
}
