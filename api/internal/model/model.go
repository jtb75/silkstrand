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
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	AgentID            *string         `json:"agent_id,omitempty"`
	Type               string          `json:"type"`
	Identifier         string          `json:"identifier"`
	Config             json.RawMessage `json:"config"`
	Environment        *string         `json:"environment,omitempty"`
	CredentialSourceID *string         `json:"credential_source_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// CredentialSource is the pluggable resolver binding from a target to a
// credential. C0 introduces only the `static` type, which wraps today's
// encrypted-at-rest credentials. See docs/adr/004-credential-resolver.md.
type CredentialSource struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

const (
	CredentialSourceTypeStatic = "static"
)

// StaticCredentialConfig is the shape of CredentialSource.Config when
// Type == "static". `EncryptedData` is a base64-encoded AES-256-GCM blob
// using the same key as the legacy credentials table.
type StaticCredentialConfig struct {
	Type          string `json:"type"`
	EncryptedData string `json:"encrypted_data"`
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
	TargetID    *string      `json:"target_id,omitempty"` // nullable since ADR 003 R0 (one-shot scans address assets)
	BundleID    string       `json:"bundle_id"`
	ScanType    string       `json:"scan_type,omitempty"` // "compliance" (default) | "discovery"
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
	ScanType string `json:"scan_type,omitempty"` // empty defaults to "compliance"
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
		TargetTypeHost, TargetTypeCIDR, TargetTypeCloud,
		TargetTypeNetworkRange:
		return true
	}
	return false
}

// ADR 003 R0 — recon pipeline types. Application code (handlers,
// store methods consuming these types) lands in subsequent PRs.

const TargetTypeNetworkRange = "network_range"

// Scan types (added by ADR 003).
const (
	ScanTypeCompliance = "compliance"
	ScanTypeDiscovery  = "discovery"
)

// DiscoveredAsset is a single (tenant, ip, port) row in the inventory.
type DiscoveredAsset struct {
	ID                string          `json:"id"`
	TenantID          string          `json:"tenant_id"`
	IP                string          `json:"ip"`
	Port              int             `json:"port"`
	Hostname          *string         `json:"hostname,omitempty"`
	Service           *string         `json:"service,omitempty"`
	Version           *string         `json:"version,omitempty"`
	Technologies      json.RawMessage `json:"technologies"`
	CVEs              json.RawMessage `json:"cves"`
	ComplianceStatus  *string         `json:"compliance_status,omitempty"`
	Source            string          `json:"source"` // 'manual' | 'discovered'
	Environment       *string         `json:"environment,omitempty"`
	FirstSeen         time.Time       `json:"first_seen"`
	LastSeen          time.Time       `json:"last_seen"`
	LastScanID        *string         `json:"last_scan_id,omitempty"`
	MissedScanCount   int             `json:"missed_scan_count"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

const (
	AssetSourceManual     = "manual"
	AssetSourceDiscovered = "discovered"
)

// AssetEvent is one append-only row in asset_events.
type AssetEvent struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	AssetID    string          `json:"asset_id"`
	ScanID     *string         `json:"scan_id,omitempty"`
	EventType  string          `json:"event_type"`
	Severity   *string         `json:"severity,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// AssetEventType enumerations (ADR 003 D4).
const (
	AssetEventNewAsset         = "new_asset"
	AssetEventAssetGone        = "asset_gone"
	AssetEventAssetReappeared  = "asset_reappeared"
	AssetEventNewCVE           = "new_cve"
	AssetEventCVEResolved      = "cve_resolved"
	AssetEventVersionChanged   = "version_changed"
	AssetEventPortOpened       = "port_opened"
	AssetEventPortClosed       = "port_closed"
	AssetEventCompliancePass   = "compliance_pass"
	AssetEventComplianceFail   = "compliance_fail"
)

// AssetSet is a saved predicate over discovered_assets (D13).
type AssetSet struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Predicate   json.RawMessage `json:"predicate"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CorrelationRule is a versioned `match → action` rule (D2).
type CorrelationRule struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenant_id"`
	Name            string          `json:"name"`
	Version         int             `json:"version"`
	Enabled         bool            `json:"enabled"`
	Trigger         string          `json:"trigger"`
	EventTypeFilter *string         `json:"event_type_filter,omitempty"`
	Body            json.RawMessage `json:"body"`
	CreatedAt       time.Time       `json:"created_at"`
	CreatedBy       *string         `json:"created_by,omitempty"`
}

const (
	RuleTriggerAssetDiscovered = "asset_discovered"
	RuleTriggerAssetEvent      = "asset_event"
)

// NotificationChannel is a per-tenant outbound channel (D12).
// Sensitive fields inside Config (webhook secret, slack webhook URL,
// pagerduty routing key) are stored as base64-encoded AES-256-GCM
// ciphertext, mirroring credential_sources.config.
type NotificationChannel struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"` // webhook | slack | email | pagerduty
	Config    json.RawMessage `json:"config"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

const (
	ChannelTypeWebhook   = "webhook"
	ChannelTypeSlack     = "slack"
	ChannelTypeEmail     = "email"
	ChannelTypePagerDuty = "pagerduty"
)

// NotificationDelivery is one append-only audit row (D12).
type NotificationDelivery struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	ChannelID    string          `json:"channel_id"`
	RuleID       *string         `json:"rule_id,omitempty"`
	EventID      *string         `json:"event_id,omitempty"`
	Severity     *string         `json:"severity,omitempty"`
	Status       string          `json:"status"`
	Attempt      int             `json:"attempt"`
	ResponseCode *int            `json:"response_code,omitempty"`
	Error        *string         `json:"error,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	DispatchedAt time.Time       `json:"dispatched_at"`
}

const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusSent      = "sent"
	DeliveryStatusFailed    = "failed"
	DeliveryStatusRetrying  = "retrying"
)

// OneShotScan is the parent record for a fan-out scan dispatch (D13).
type OneShotScan struct {
	ID               string          `json:"id"`
	TenantID         string          `json:"tenant_id"`
	BundleID         string          `json:"bundle_id"`
	AssetSetID       *string         `json:"asset_set_id,omitempty"`
	InlinePredicate  json.RawMessage `json:"inline_predicate,omitempty"`
	MaxConcurrency   int             `json:"max_concurrency"`
	RateLimitPPS     *int            `json:"rate_limit_pps,omitempty"`
	TotalTargets     *int            `json:"total_targets,omitempty"`
	CompletedTargets int             `json:"completed_targets"`
	Status           string          `json:"status"`
	TriggeredBy      *string         `json:"triggered_by,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	DispatchedAt     *time.Time      `json:"dispatched_at,omitempty"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}
