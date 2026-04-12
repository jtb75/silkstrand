package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	RedisURL                string
	JWTSecret               string
	ClerkJWKSURL            string // If set, validates Clerk JWTs via JWKS; if empty, uses HMAC-SHA256
	ClerkIssuerURL          string // Expected issuer (iss) claim in Clerk JWTs
	InternalAPIKey          string
	CredentialEncryptionKey []byte   // 32 bytes for AES-256-GCM
	AllowedOrigins         []string // Allowed WebSocket origins (empty = allow all in dev)
}

func Load() (*Config, error) {
	var credKey []byte
	if credKeyHex := getEnv("CREDENTIAL_ENCRYPTION_KEY", ""); credKeyHex != "" {
		var err error
		credKey, err = hex.DecodeString(credKeyHex)
		if err != nil {
			return nil, fmt.Errorf("decoding CREDENTIAL_ENCRYPTION_KEY: %w", err)
		}
		if len(credKey) != 32 {
			return nil, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(credKey))
		}
	}

	cfg := &Config{
		Port:                    getEnv("PORT", "8080"),
		DatabaseURL:             getEnv("DATABASE_URL", "postgres://silkstrand:localdev@localhost:5432/silkstrand?sslmode=disable"),
		RedisURL:                getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:               getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		ClerkJWKSURL:            getEnv("CLERK_JWKS_URL", ""),
		ClerkIssuerURL:          getEnv("CLERK_ISSUER_URL", ""),
		InternalAPIKey:          getEnv("INTERNAL_API_KEY", ""),
		CredentialEncryptionKey: credKey,
		AllowedOrigins:         parseOrigins(getEnv("ALLOWED_ORIGINS", "")),
	}

	if getEnv("ENV", "dev") == "production" {
		if cfg.JWTSecret == "dev-secret-change-in-production" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		if len(cfg.CredentialEncryptionKey) == 0 {
			return nil, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY must be set in production")
		}
		if cfg.InternalAPIKey == "" {
			return nil, fmt.Errorf("INTERNAL_API_KEY must be set in production")
		}
	}

	return cfg, nil
}

func parseOrigins(s string) []string {
	if s == "" {
		return nil
	}
	origins := strings.Split(s, ",")
	for i := range origins {
		origins[i] = strings.TrimSpace(origins[i])
	}
	return origins
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
