package middleware

import (
	"crypto/subtle"
	"net/http"
)

// InternalAuth validates the X-API-Key header for internal API routes
// used by the backoffice to manage this data center.
func InternalAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				http.Error(w, `{"error":"internal API not configured"}`, http.StatusServiceUnavailable)
				return
			}

			key := r.Header.Get("X-API-Key")
			if key == "" {
				http.Error(w, `{"error":"API key required"}`, http.StatusUnauthorized)
				return
			}

			if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
