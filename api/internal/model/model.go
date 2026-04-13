package model

import (
	"encoding/json"
	"time"
)

type Tenant struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
}

const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusInactive  = "inactive"
)

type DCStats struct {
	TenantCount int `json:"tenant_count"`
	AgentCount  int `json:"agent_count"`
	ScanCount   int `json:"scan_count"`
}

type CreateTenantRequest struct {
	Name string `json:"name"`
}

type UpdateTenantRequest struct {
	Name   *string         `json:"name,omitempty"`
	Status *string         `json:"status,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
}

type Agent struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	Name          string     `json:"name"`
	Status        string     `json:"status"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Version       *string    `json:"version,omitempty"`
	KeyHash       string     `json:"-"`
	NextKeyHash   *string    `json:"-"`
	KeyRotatedAt  *time.Time `json:"key_rotated_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type CreateAgentRequest struct {
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type Bundle struct {
	ID         string     `json:"id"`
	TenantID   *string    `json:"tenant_id,omitempty"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	Framework  string     `json:"framework"`
	TargetType string     `json:"target_type"`
	GCSPath    *string    `json:"gcs_path,omitempty"`
	Signature  *string    `json:"signature,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Target struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	AgentID     *string         `json:"agent_id,omitempty"`
	Type        string          `json:"type"`
	Identifier  string          `json:"identifier"`
	Config      json.RawMessage `json:"config"`
	Environment *string         `json:"environment,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type CreateTargetRequest struct {
	AgentID     *string         `json:"agent_id,omitempty"`
	Type        string          `json:"type"`
	Identifier  string          `json:"identifier"`
	Config      json.RawMessage `json:"config,omitempty"`
	Environment string          `json:"environment,omitempty"`
}

type UpdateTargetRequest struct {
	AgentID     *string         `json:"agent_id,omitempty"`
	Type        *string         `json:"type,omitempty"`
	Identifier  *string         `json:"identifier,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
	Environment *string         `json:"environment,omitempty"`
}

type Scan struct {
	ID          string       `json:"id"`
	TenantID    string       `json:"tenant_id"`
	AgentID     *string      `json:"agent_id,omitempty"`
	TargetID    string       `json:"target_id"`
	BundleID    string       `json:"bundle_id"`
	Status      string       `json:"status"`
	StartedAt   *time.Time   `json:"started_at,omitempty"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	Results     []ScanResult `json:"results,omitempty"`
	Summary     *ScanSummary `json:"summary,omitempty"`
}

type ScanSummary struct {
	Total         int `json:"total"`
	Pass          int `json:"pass"`
	Fail          int `json:"fail"`
	Error         int `json:"error"`
	NotApplicable int `json:"not_applicable"`
}

type CreateScanRequest struct {
	TargetID string `json:"target_id"`
	BundleID string `json:"bundle_id"`
}

type ScanResult struct {
	ID          string          `json:"id"`
	ScanID      string          `json:"scan_id"`
	ControlID   string          `json:"control_id"`
	Title       string          `json:"title"`
	Status      string          `json:"status"`
	Severity    string          `json:"severity,omitempty"`
	Evidence    json.RawMessage `json:"evidence,omitempty"`
	Remediation string          `json:"remediation,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Scan statuses
const (
	ScanStatusPending   = "pending"
	ScanStatusRunning   = "running"
	ScanStatusCompleted = "completed"
	ScanStatusFailed    = "failed"
)

// Agent statuses
const (
	AgentStatusConnected    = "connected"
	AgentStatusDisconnected = "disconnected"
)

// Target types — engine-specific. Historic value "database" was Postgres only
// and is migrated to "postgresql" in migration 009.
const (
	TargetTypePostgreSQL       = "postgresql"
	TargetTypeAuroraPostgreSQL = "aurora_postgresql"
	TargetTypeMSSQL            = "mssql"
	TargetTypeMongoDB          = "mongodb"
	TargetTypeMySQL            = "mysql"
	TargetTypeAuroraMySQL      = "aurora_mysql"
	TargetTypeHost             = "host"
	TargetTypeCIDR             = "cidr"
	TargetTypeCloud            = "cloud"
)

// IsValidTargetType returns true for known engine/type identifiers.
func IsValidTargetType(t string) bool {
	switch t {
	case TargetTypePostgreSQL, TargetTypeAuroraPostgreSQL,
		TargetTypeMSSQL, TargetTypeMongoDB,
		TargetTypeMySQL, TargetTypeAuroraMySQL,
		TargetTypeHost, TargetTypeCIDR, TargetTypeCloud:
		return true
	}
	return false
}
