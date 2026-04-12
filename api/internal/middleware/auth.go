package middleware

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Auth validates JWT tokens from the Authorization header.
// If clerkJWKSURL is set, it validates Clerk-issued JWTs using JWKS (RS256).
// If clerkJWKSURL is empty, it falls back to HMAC-SHA256 signed JWTs (dev mode).
func Auth(jwtSecret string, clerkJWKSURL string) func(http.Handler) http.Handler {
	var jwks *jwksCache
	if clerkJWKSURL != "" {
		jwks = newJWKSCache(clerkJWKSURL)
		slog.Info("auth mode: Clerk JWKS", "jwks_url", clerkJWKSURL)
	} else {
		slog.Info("auth mode: HMAC-SHA256 (dev)")
	}

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

			var claims *Claims
			var err error

			if jwks != nil {
				claims, err = validateClerkJWT(parts[1], jwks)
			} else {
				claims, err = validateHMACJWT(parts[1], jwtSecret)
			}

			if err != nil {
				slog.Debug("token validation failed", "error", err)
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

// validateHMACJWT validates a JWT signed with HMAC-SHA256 (dev mode).
func validateHMACJWT(token, secret string) (*Claims, error) {
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

// validateClerkJWT validates a Clerk-issued JWT using JWKS (RS256).
func validateClerkJWT(token string, cache *jwksCache) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidToken
	}

	// Parse header to get kid
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decoding header: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parsing header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	// Get the RSA public key from JWKS
	pubKey, err := cache.getKey(header.Kid)
	if err != nil {
		return nil, fmt.Errorf("getting JWKS key: %w", err)
	}

	// Verify RS256 signature
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decoding signature: %w", err)
	}

	signedContent := []byte(parts[0] + "." + parts[1])
	h := sha256.Sum256(signedContent)

	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, h[:], sigBytes); err != nil {
		return nil, errInvalidToken
	}

	// Decode claims
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	// Clerk JWT claims structure
	var rawClaims struct {
		Sub      string `json:"sub"`
		Exp      int64  `json:"exp"`
		Metadata *struct {
			TenantID string `json:"tenant_id"`
		} `json:"metadata"`
		PublicMetadata *struct {
			TenantID string `json:"tenant_id"`
		} `json:"public_metadata"`
	}
	if err := json.Unmarshal(payload, &rawClaims); err != nil {
		return nil, errInvalidToken
	}

	if rawClaims.Exp > 0 && time.Now().Unix() > rawClaims.Exp {
		return nil, errTokenExpired
	}

	// Extract tenant_id from metadata or public_metadata
	tenantID := ""
	if rawClaims.Metadata != nil && rawClaims.Metadata.TenantID != "" {
		tenantID = rawClaims.Metadata.TenantID
	} else if rawClaims.PublicMetadata != nil && rawClaims.PublicMetadata.TenantID != "" {
		tenantID = rawClaims.PublicMetadata.TenantID
	}

	if tenantID == "" {
		return nil, fmt.Errorf("missing tenant_id in Clerk JWT metadata")
	}

	return &Claims{
		TenantID: tenantID,
		UserID:   rawClaims.Sub,
		Exp:      rawClaims.Exp,
	}, nil
}

// jwksCache fetches and caches JWKS keys from a URL.
type jwksCache struct {
	url     string
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
	ttl     time.Duration
}

func newJWKSCache(url string) *jwksCache {
	return &jwksCache{
		url:  url,
		keys: make(map[string]*rsa.PublicKey),
		ttl:  1 * time.Hour,
	}
}

func (c *jwksCache) getKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, ok := c.keys[kid]
	expired := time.Since(c.fetched) > c.ttl
	c.mu.RUnlock()

	if ok && !expired {
		return key, nil
	}

	// Fetch fresh JWKS (also handles unknown kid after cache refresh)
	if err := c.refresh(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	key, ok = c.keys[kid]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("key %q not found in JWKS", kid)
	}
	return key, nil
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

func (c *jwksCache) refresh() error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(c.url)
	if err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	if err != nil {
		return fmt.Errorf("reading JWKS response: %w", err)
	}

	var jwksResp jwksResponse
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return fmt.Errorf("parsing JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwksResp.Keys))
	for _, k := range jwksResp.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pubKey, err := parseRSAPublicKey(k.N, k.E)
		if err != nil {
			slog.Warn("skipping JWKS key", "kid", k.Kid, "error", err)
			continue
		}
		keys[k.Kid] = pubKey
	}

	c.mu.Lock()
	c.keys = keys
	c.fetched = time.Now()
	c.mu.Unlock()

	slog.Info("JWKS cache refreshed", "keys", len(keys))
	return nil
}

// parseRSAPublicKey builds an *rsa.PublicKey from base64url-encoded n and e values.
func parseRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decoding modulus: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decoding exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	if !e.IsInt64() {
		return nil, fmt.Errorf("exponent too large")
	}

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

type errType string

func (e errType) Error() string { return string(e) }

const (
	errInvalidToken errType = "invalid token"
	errTokenExpired errType = "token expired"
)
