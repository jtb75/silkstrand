// Package prober performs lightweight connectivity checks against a target,
// independent of the scan bundle pipeline. Used by the "Test connection"
// button in the tenant UI. MVP supports database (PostgreSQL) targets; other
// target types return an explicit "not implemented" result.
package prober

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DatabaseConfig is the subset of target config the prober cares about.
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	SSLMode  string `json:"sslmode"`
}

// DatabaseCredentials mirrors what the UI credential form saves.
type DatabaseCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Result is a compact outcome for the UI.
type Result struct {
	OK     bool
	Error  string
	Detail string // e.g. "PostgreSQL 16.13 on aarch64-apple-darwin…"
}

// Probe dispatches on target type. Times out after 10s regardless.
func Probe(ctx context.Context, targetType string, configRaw, credsRaw json.RawMessage) Result {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch targetType {
	case "database":
		return probeDatabase(ctx, configRaw, credsRaw)
	default:
		return Result{OK: false, Error: fmt.Sprintf("connectivity test not implemented for target type %q", targetType)}
	}
}

func probeDatabase(ctx context.Context, configRaw, credsRaw json.RawMessage) Result {
	var cfg DatabaseConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return Result{OK: false, Error: "invalid target config: " + err.Error()}
	}
	if cfg.Host == "" {
		return Result{OK: false, Error: "target config missing 'host'"}
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "prefer"
	}
	if cfg.Database == "" {
		cfg.Database = "postgres"
	}

	var creds DatabaseCredentials
	if len(credsRaw) > 0 && string(credsRaw) != "null" {
		if err := json.Unmarshal(credsRaw, &creds); err != nil {
			return Result{OK: false, Error: "invalid credentials: " + err.Error()}
		}
	}
	if creds.Username == "" {
		return Result{OK: false, Error: "no credential set for this target (Targets → Credential)"}
	}

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=8",
		cfg.Host, cfg.Port, cfg.Database, creds.Username, creds.Password, cfg.SSLMode)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return Result{OK: false, Error: err.Error()}
	}
	defer conn.Close(ctx)

	var version string
	if err := conn.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return Result{OK: false, Error: "connected but version query failed: " + err.Error()}
	}
	return Result{OK: true, Detail: version}
}
