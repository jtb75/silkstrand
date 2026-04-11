package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Auth validates JWT tokens from the Authorization header.
// MVP: HMAC-SHA256 signed JWTs. Post-MVP: swap for Auth0/Clerk JWKS validation.
func Auth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := validateJWT(parts[1], jwtSecret)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := SetClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type Claims struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Exp      int64  `json:"exp"`
}

func validateJWT(token, secret string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidToken
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errInvalidToken
	}

	// Decode claims
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errInvalidToken
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, errTokenExpired
	}

	if claims.TenantID == "" {
		return nil, errInvalidToken
	}

	return &claims, nil
}

type errType string

func (e errType) Error() string { return string(e) }

const (
	errInvalidToken errType = "invalid token"
	errTokenExpired errType = "token expired"
)
