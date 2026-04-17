package bundler

import (
	"crypto/hmac"
	"crypto/sha256"
)

// hmacSHA256 computes an HMAC-SHA256 signature of data using key.
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
