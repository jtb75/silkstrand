package config

import (
	"encoding/hex"
	"fmt"
	"os"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	EncryptionKey []byte // 32 bytes for AES-256-GCM
}

func Load() (*Config, error) {
	var encKey []byte
	if encKeyHex := getEnv("ENCRYPTION_KEY", ""); encKeyHex != "" {
		var err error
		encKey, err = hex.DecodeString(encKeyHex)
		if err != nil {
			return nil, fmt.Errorf("decoding ENCRYPTION_KEY: %w", err)
		}
		if len(encKey) != 32 {
			return nil, fmt.Errorf("ENCRYPTION_KEY must be 32 bytes (64 hex chars), got %d bytes", len(encKey))
		}
	}

	cfg := &Config{
		Port:          getEnv("PORT", "8081"),
		DatabaseURL:   getEnv("DATABASE_URL", "postgres://silkstrand:localdev@localhost:15433/silkstrand_backoffice?sslmode=disable"),
		JWTSecret:     getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		EncryptionKey: encKey,
	}

	if getEnv("ENV", "dev") == "production" {
		if cfg.JWTSecret == "dev-secret-change-in-production" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		if len(cfg.EncryptionKey) == 0 {
			return nil, fmt.Errorf("ENCRYPTION_KEY must be set in production")
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
