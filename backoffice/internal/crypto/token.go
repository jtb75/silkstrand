package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// NewURLToken returns a cryptographically random 256-bit token, URL-safe
// base64-encoded without padding (43 chars). Used for invitation links and
// password reset links.
//
// Returns (plaintext, sha256hash). Store only the hash; email the plaintext.
func NewURLToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("reading random bytes: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(plaintext))
	return plaintext, h[:], nil
}

// HashToken returns the SHA-256 hash of the supplied plaintext URL token.
// Use this when a user presents a token so you can look it up by hash.
func HashToken(plaintext string) []byte {
	h := sha256.Sum256([]byte(plaintext))
	return h[:]
}
