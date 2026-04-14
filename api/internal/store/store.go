package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// Store defines the data access interface. All methods are tenant-scoped
// via the tenant ID stored in the context.
type Store interface {
	// Targets
	ListTargets(ctx context.Context) ([]model.Target, error)
	GetTarget(ctx context.Context, id string) (*model.Target, error)
	CreateTarget(ctx context.Context, req model.CreateTargetRequest) (*model.Target, error)
	UpdateTarget(ctx context.Context, id string, req model.UpdateTargetRequest) (*model.Target, error)
	DeleteTarget(ctx context.Context, id string) error

	// Scans
	ListScans(ctx context.Context) ([]model.Scan, error)
	GetScan(ctx context.Context, id string) (*model.Scan, error)
	CreateScan(ctx context.Context, req model.CreateScanRequest) (*model.Scan, error)
	UpdateScanStatus(ctx context.Context, id string, status string) error
	DeleteScan(ctx context.Context, id string) error

	// Scan Results
	CreateScanResults(ctx context.Context, scanID string, results []model.ScanResult) error
	GetScanResults(ctx context.Context, scanID string) ([]model.ScanResult, error)

	// Agents
	GetAgent(ctx context.Context, id string) (*model.Agent, error)
	GetAgentByID(ctx context.Context, id string) (*model.Agent, error) // not tenant-scoped, for WSS auth
	CreateAgent(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, string, error)
	UpdateAgentStatus(ctx context.Context, id string, status string) error
	UpdateAgentHeartbeat(ctx context.Context, id, version string) error
	RotateAgentKey(ctx context.Context, id string) (string, error)
	PromoteAgentKey(ctx context.Context, id string) error
	ListAgents(ctx context.Context) ([]model.Agent, error)
	DeleteAgent(ctx context.Context, id string) error

	// Install tokens (one-time bootstrap credentials)
	CreateInstallToken(ctx context.Context, tenantID string, tokenHash []byte, expiresAt time.Time, createdBy string) error
	ConsumeInstallToken(ctx context.Context, tokenHash []byte, agentID string) (tenantID string, err error)

	// Targets (non-tenant-scoped, for directive enrichment)
	GetTargetByID(ctx context.Context, id string) (*model.Target, error)

	// Scans (non-tenant-scoped, for agent message ingest)
	GetScanByID(ctx context.Context, id string) (*model.Scan, error)

	// Bundles
	GetBundle(ctx context.Context, id string) (*model.Bundle, error)
	ListBundlesForTenant(ctx context.Context, tenantID string) ([]model.Bundle, error)
	UpsertBundle(ctx context.Context, b model.Bundle) (*model.Bundle, error)

	// Credentials (legacy table; kept authoritative through ADR 004 C0
	// rollback window — writes dual-write to both surfaces).
	GetCredentialsByTarget(ctx context.Context, targetID string) (json.RawMessage, error)
	CreateCredential(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) (string, error)
	UpsertCredential(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) error
	DeleteCredential(ctx context.Context, tenantID, targetID string) error
	HasCredential(ctx context.Context, targetID string) (bool, string, error)

	// Credential Sources (ADR 004 C0).
	CreateCredentialSource(ctx context.Context, tenantID, srcType string, config json.RawMessage) (string, error)
	GetCredentialSource(ctx context.Context, id string) (*model.CredentialSource, error)
	GetCredentialSourceByTarget(ctx context.Context, targetID string) (*model.CredentialSource, error)
	UpdateCredentialSourceConfig(ctx context.Context, id string, config json.RawMessage) error
	DeleteCredentialSource(ctx context.Context, id string) error
	SetTargetCredentialSource(ctx context.Context, targetID, sourceID string) error
	ClearTargetCredentialSource(ctx context.Context, targetID string) error

	// UpsertStaticCredentialSource ensures a `static`-type credential_sources
	// row exists for the target, pointed at by targets.credential_source_id,
	// carrying the given type + AES-GCM encrypted blob. Idempotent.
	UpsertStaticCredentialSource(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) error

	// GetStaticCredentialForTarget resolves the credential bytes + type for a
	// target, preferring credential_sources (type=static) and falling back to
	// the legacy credentials table. Returns (nil, "", nil) when neither is set.
	GetStaticCredentialForTarget(ctx context.Context, targetID string) ([]byte, string, error)

	// HasCredentialForTarget is the read-path equivalent of HasCredential,
	// preferring credential_sources with legacy fallback.
	HasCredentialForTarget(ctx context.Context, targetID string) (bool, string, error)

	// DeleteCredentialForTarget removes both the credential_sources row (if
	// linked via static) and the legacy credentials row. Returns sql.ErrNoRows
	// if nothing existed in either place.
	DeleteCredentialForTarget(ctx context.Context, tenantID, targetID string) error

	// Tenants (internal, not tenant-scoped)
	CreateTenant(ctx context.Context, name string) (*model.Tenant, error)
	ListAllTenants(ctx context.Context) ([]model.Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*model.Tenant, error)
	UpdateTenantStatus(ctx context.Context, id string, status string) error
	UpdateTenantConfig(ctx context.Context, id string, config json.RawMessage) error
	UpdateTenantName(ctx context.Context, id string, name string) error

	// Agents (internal, cross-tenant)
	ListAllAgents(ctx context.Context) ([]model.Agent, error)

	// Scans (internal)
	FailRunningScansForAgent(ctx context.Context, agentID string) (int, error)

	// Stats
	GetDCStats(ctx context.Context) (*model.DCStats, error)

	// Health
	Ping(ctx context.Context) error

	// Recon (ADR 003 R1a) — discovered_assets + asset_events.
	UpsertDiscoveredAsset(ctx context.Context, scanID string, in DiscoveredAssetInput) (newAsset *model.DiscoveredAsset, oldAsset *model.DiscoveredAsset, err error)
	AppendAssetEvents(ctx context.Context, events []model.AssetEvent) error
	GetAssetByID(ctx context.Context, id string) (*model.DiscoveredAsset, error)
	ListAssets(ctx context.Context, filter AssetFilter) (items []model.DiscoveredAsset, total int, err error)
	ListAssetEventsByAsset(ctx context.Context, assetID string, limit int) ([]model.AssetEvent, error)
	UpsertManualAsset(ctx context.Context, tenantID, ip string, port int, environment *string) (*model.DiscoveredAsset, error)
	SetTargetAsset(ctx context.Context, targetID, assetID string) error
}

// DiscoveredAssetInput is what the agent's asset_discovered payload
// becomes after parsing. The store upserts on (tenant_id, ip, port).
type DiscoveredAssetInput struct {
	TenantID     string
	IP           string
	Port         int
	Hostname     string
	Service      string
	Version      string
	Technologies json.RawMessage
	CVEs         json.RawMessage
	Environment  *string
}

// AssetFilter is the parsed query for ListAssets. All zero-value
// fields are ignored. R1a ships canned filter chips on top of this.
type AssetFilter struct {
	Service          string
	ServiceIn        []string
	IPCIDR           string
	HasCVECountGTE   int
	Source           string
	ComplianceStatus string
	NewSinceDuration time.Duration // first_seen >= now - dur
	ChangedSinceDuration time.Duration // last_seen >= now - dur
	Q                string
	SortBy           string // last_seen|first_seen|ip|service|cve_count
	SortDesc         bool
	Page             int
	PageSize         int
}

type contextKey string

const TenantIDKey contextKey = "tenant_id"

// TenantID extracts the tenant ID from the context.
func TenantID(ctx context.Context) string {
	v, _ := ctx.Value(TenantIDKey).(string)
	return v
}

// WithTenantID returns a new context with the tenant ID set.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}
