package config

import (
	"encoding/hex"
	"fmt"
	"os"
)

type Config struct {
	Port                    string
	DatabaseURL             string
	RedisURL                string
	JWTSecret               string
	InternalAPIKey          string
	CredentialEncryptionKey []byte // 32 bytes for AES-256-GCM
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
		InternalAPIKey:          getEnv("INTERNAL_API_KEY", ""),
		CredentialEncryptionKey: credKey,
	}

	if cfg.JWTSecret == "dev-secret-change-in-production" && getEnv("ENV", "dev") == "production" {
		return nil, fmt.Errorf("JWT_SECRET must be set in production")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
