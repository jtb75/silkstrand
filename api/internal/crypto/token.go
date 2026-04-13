package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// NewInstallToken returns a cryptographically random 256-bit token, URL-safe
// base64 without padding (43 chars), plus its SHA-256 hash. Store the hash;
// hand the plaintext to the admin exactly once.
func NewInstallToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("reading random bytes: %w", err)
	}
	plaintext := "sst_" + base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(plaintext))
	return plaintext, h[:], nil
}

// HashInstallToken returns the SHA-256 hash of a plaintext install token.
func HashInstallToken(plaintext string) []byte {
	h := sha256.Sum256([]byte(plaintext))
	return h[:]
}
