package config

import (
	"fmt"
	"log/slog"
	"os"
)

// Config holds the agent configuration loaded from environment variables.
type Config struct {
	APIURL    string // env: SILKSTRAND_API_URL, default "ws://localhost:8080"
	AgentID   string // env: SILKSTRAND_AGENT_ID, required
	AgentKey  string // env: SILKSTRAND_AGENT_KEY, required
	BundleDir string // env: SILKSTRAND_BUNDLE_DIR, default "./bundles"
	LogLevel  string // env: SILKSTRAND_LOG_LEVEL, default "info"
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		APIURL:    envOrDefault("SILKSTRAND_API_URL", "ws://localhost:8080"),
		AgentID:   os.Getenv("SILKSTRAND_AGENT_ID"),
		AgentKey:  os.Getenv("SILKSTRAND_AGENT_KEY"),
		BundleDir: envOrDefault("SILKSTRAND_BUNDLE_DIR", "./bundles"),
		LogLevel:  envOrDefault("SILKSTRAND_LOG_LEVEL", "info"),
	}

	if cfg.AgentID == "" {
		return nil, fmt.Errorf("SILKSTRAND_AGENT_ID is required")
	}
	if cfg.AgentKey == "" {
		return nil, fmt.Errorf("SILKSTRAND_AGENT_KEY is required")
	}

	return cfg, nil
}

// ParseLogLevel converts the string log level to a slog.Level.
func ParseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
