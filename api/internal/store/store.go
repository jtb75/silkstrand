// Package store defines the data access interface. All tenant-scoped
// methods read the tenant id from context (see TenantID). Non-tenant-scoped
// methods are called out explicitly in the godoc for each method.
//
// This file is the P1 asset-first refactor shape: the recon pipeline
// model (DiscoveredAsset / AssetSet / OneShotScan / NotificationChannel)
// is gone; Asset / AssetEndpoint / Collection / Finding / ScanDefinition
// are its successors. Real implementations live in postgres.go; methods
// that return errNotImplemented are P2+ work per the execution plan.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
)

// ErrNotImplemented is returned from store methods that are part of the
// asset-first surface but whose implementations land in later phases
// (P2: ingest + rules, P3: findings + scheduler, P4: collections query).
var ErrNotImplemented = errors.New("store method not implemented in P1")

// Store defines the data access interface.
type Store interface {
	// --- Platform ---------------------------------------------------

	// Targets (CIDR / network_range only post-ADR-006 D8).
	ListTargets(ctx context.Context) ([]model.Target, error)
	GetTarget(ctx context.Context, id string) (*model.Target, error)
	GetTargetByID(ctx context.Context, id string) (*model.Target, error) // not tenant-scoped
	CreateTarget(ctx context.Context, req model.CreateTargetRequest) (*model.Target, error)
	UpdateTarget(ctx context.Context, id string, req model.UpdateTargetRequest) (*model.Target, error)
	DeleteTarget(ctx context.Context, id string) error
	// UpsertTargetByCIDR looks up (or creates) a targets row matching the
	// tenant + CIDR tuple, returning its id. Used by the scheduler's
	// CIDR-scope dispatch to materialize a target row so the agent
	// receives a directive with target_type='cidr' + identifier=<cidr>.
	// Relies on the partial unique index from migration 019.
	UpsertTargetByCIDR(ctx context.Context, tenantID, cidr string, agentID *string, environment string) (string, error)

	// Agents
	GetAgent(ctx context.Context, id string) (*model.Agent, error)
	GetAgentByID(ctx context.Context, id string) (*model.Agent, error)
	CreateAgent(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, string, error)
	UpdateAgentStatus(ctx context.Context, id, status string) error
	UpdateAgentHeartbeat(ctx context.Context, id, version string) error
	RotateAgentKey(ctx context.Context, id string) (string, error)
	PromoteAgentKey(ctx context.Context, id string) error
	ListAgents(ctx context.Context) ([]model.Agent, error)
	ListAllAgents(ctx context.Context) ([]model.Agent, error)
	DeleteAgent(ctx context.Context, id string) error

	// Install tokens
	CreateInstallToken(ctx context.Context, tenantID string, tokenHash []byte, expiresAt time.Time, createdBy string) error
	ConsumeInstallToken(ctx context.Context, tokenHash []byte, agentID string) (tenantID string, err error)

	// Bundles
	GetBundle(ctx context.Context, id string) (*model.Bundle, error)
	ListBundlesForTenant(ctx context.Context, tenantID string) ([]model.Bundle, error)
	UpsertBundle(ctx context.Context, b model.Bundle) (*model.Bundle, error)

	// Tenants (internal)
	CreateTenant(ctx context.Context, name string) (*model.Tenant, error)
	ListAllTenants(ctx context.Context) ([]model.Tenant, error)
	GetTenantByID(ctx context.Context, id string) (*model.Tenant, error)
	UpdateTenantStatus(ctx context.Context, id, status string) error
	UpdateTenantConfig(ctx context.Context, id string, config json.RawMessage) error
	UpdateTenantName(ctx context.Context, id, name string) error

	// Stats
	GetDCStats(ctx context.Context) (*model.DCStats, error)

	// Health
	Ping(ctx context.Context) error

	// --- Credential sources (ADR 004 C0) ----------------------------

	CreateCredentialSource(ctx context.Context, tenantID, srcType string, config json.RawMessage) (string, error)
	GetCredentialSource(ctx context.Context, id string) (*model.CredentialSource, error)
	GetCredentialSourceByTarget(ctx context.Context, targetID string) (*model.CredentialSource, error)
	ListCredentialSources(ctx context.Context, tenantID string) ([]model.CredentialSource, error)
	UpdateCredentialSourceConfig(ctx context.Context, id string, config json.RawMessage) error
	DeleteCredentialSource(ctx context.Context, id string) error

	// Credential mappings (ADR 006 P6 — collection ↔ credential_source).
	ListCredentialMappings(ctx context.Context, tenantID string) ([]model.CredentialMapping, error)
	GetCredentialMapping(ctx context.Context, id string) (*model.CredentialMapping, error)
	CreateCredentialMapping(ctx context.Context, tenantID, collectionID, credentialSourceID string) (*model.CredentialMapping, error)
	DeleteCredentialMapping(ctx context.Context, id string) error
	CountMappingsForSource(ctx context.Context, sourceID string) (int, error)
	SetTargetCredentialSource(ctx context.Context, targetID, sourceID string) error
	ClearTargetCredentialSource(ctx context.Context, targetID string) error
	UpsertStaticCredentialSource(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) error
	GetStaticCredentialForTarget(ctx context.Context, targetID string) ([]byte, string, error)
	HasCredentialForTarget(ctx context.Context, targetID string) (bool, string, error)
	DeleteCredentialForTarget(ctx context.Context, tenantID, targetID string) error

	// --- Scans (execution history, ADR 007 D3) ----------------------
	//
	// The ad-hoc CreateScan surface survives as an internal helper for
	// the scheduler (P3) and the existing `POST /api/v1/scans` route.
	// P1 ships a working write path because the Scan handler still
	// lives (it returns 501 on all non-bundle routes; the list/get
	// surface otherwise compiles empty).

	ListScans(ctx context.Context) ([]model.Scan, error)
	GetScan(ctx context.Context, id string) (*model.Scan, error)
	GetScanByID(ctx context.Context, id string) (*model.Scan, error) // not tenant-scoped
	CreateScan(ctx context.Context, req model.CreateScanRequest) (*model.Scan, error)
	UpdateScanStatus(ctx context.Context, id, status string) error
	FailScan(ctx context.Context, id, reason string) error
	DeleteScan(ctx context.Context, id string) error
	FailRunningScansForAgent(ctx context.Context, agentID string) (int, error)
	AgentHasRunningScan(ctx context.Context, agentID string) (bool, error)
	OldestQueuedScanForAgent(ctx context.Context, agentID string) (*model.Scan, error)
	FailStaleQueuedScans(ctx context.Context, maxAge time.Duration) (int, error)

	// --- Assets + endpoints (ADR 006 D2) ----------------------------
	//
	// P1 ships minimal working impls for the read side + a write path
	// that ingest (P2) will call. The field set is narrow until P2.

	UpsertAsset(ctx context.Context, in UpsertAssetInput) (*model.Asset, error)
	UpsertAssetEndpoint(ctx context.Context, in UpsertAssetEndpointInput) (*model.AssetEndpoint, error)
	GetAssetByID(ctx context.Context, id string) (*model.Asset, error)
	ListAssets(ctx context.Context, filter AssetFilter) (items []model.Asset, total int, err error)

	// --- Collections (ADR 006 D5) -----------------------------------

	CreateCollection(ctx context.Context, c model.Collection) (*model.Collection, error)
	GetCollection(ctx context.Context, id string) (*model.Collection, error)
	ListCollections(ctx context.Context) ([]model.Collection, error)
	UpdateCollection(ctx context.Context, c model.Collection) (*model.Collection, error)
	DeleteCollection(ctx context.Context, id string) error

	// --- P2 ingest surface ------------------------------------------
	//
	// Discovery ingest path uses these in addition to UpsertAsset /
	// UpsertAssetEndpoint above. AppendAssetEvents is partition-aware
	// via the occurred_at column. RecordDiscoverySource logs provenance
	// per ADR 006 D9.
	AppendAssetEvents(ctx context.Context, events []model.AssetEvent) error
	RecordDiscoverySource(ctx context.Context, in DiscoverySourceInput) error
	ListEndpointsForAsset(ctx context.Context, assetID string) ([]model.AssetEndpoint, error)
	UpdateEndpointAllowlistStatus(ctx context.Context, endpointID, status string) error

	// Rule-engine loader.
	ListActiveRulesForTrigger(ctx context.Context, tenantID, trigger string) ([]model.CorrelationRule, error)

	// Correlation-rule CRUD (ADR 006 D6; P4 wires the collection-aware
	// matcher). Tenant-scoped.
	ListCorrelationRules(ctx context.Context) ([]model.CorrelationRule, error)
	GetCorrelationRule(ctx context.Context, id string) (*model.CorrelationRule, error)
	CreateCorrelationRule(ctx context.Context, r model.CorrelationRule) (*model.CorrelationRule, error)
	UpdateCorrelationRule(ctx context.Context, r model.CorrelationRule) (*model.CorrelationRule, error)
	DeleteCorrelationRule(ctx context.Context, id string) error

	// P4 read surface for preview / members / coverage roll-ups.
	ListAllAssetsTenant(ctx context.Context) ([]model.Asset, error)
	ListEndpointsForAssetTenant(ctx context.Context, assetID string) ([]model.AssetEndpoint, error)
	GetAssetEndpointByID(ctx context.Context, endpointID string) (*model.AssetEndpoint, *model.Asset, error)
	ListAllEndpointViewsTenant(ctx context.Context) ([]EndpointRow, error)
	ListAllFindingsTenant(ctx context.Context) ([]model.Finding, error)
	ListFindingsForEndpoint(ctx context.Context, endpointID string) ([]model.Finding, error)

	// Coverage helpers.
	CountEndpointsByAsset(ctx context.Context) (map[string]int, error)
	EndpointsWithScanDefinitionByAsset(ctx context.Context) (map[string]map[string]struct{}, error)
	LastScanAtByAsset(ctx context.Context) (map[string]time.Time, error)
	NextScanAtByAsset(ctx context.Context) (map[string]time.Time, error)
	FindingsSeverityByEndpoint(ctx context.Context) (map[string]map[string]int, error)
	CollectionsWithCredentialMappings(ctx context.Context) ([]model.Collection, error)

	// Agent allowlist snapshot (ADR 003 D11 — restored in migration 018).
	UpsertAgentAllowlist(ctx context.Context, in AgentAllowlistInput) (changed bool, err error)
	GetAgentAllowlist(ctx context.Context, agentID string) (*AgentAllowlistSnapshot, error)

	// Notification deliveries (ADR 006 P6 — channel_source_id → credential_sources).
	InsertNotificationDelivery(ctx context.Context, d model.NotificationDelivery) error

	// --- Findings (ADR 007 D1/D2/D7) --------------------------------

	UpsertFinding(ctx context.Context, in UpsertFindingInput) (*model.Finding, error)
	ListFindings(ctx context.Context, f FindingFilter) ([]model.Finding, error)
	GetFindingByID(ctx context.Context, id string) (*model.Finding, error)
	SetFindingStatus(ctx context.Context, id, status string) error

	// --- Scan definitions (ADR 007 D3/D4/D5) ------------------------

	CreateScanDefinition(ctx context.Context, in model.ScanDefinition) (*model.ScanDefinition, error)
	GetScanDefinition(ctx context.Context, id string) (*model.ScanDefinition, error)
	ListScanDefinitions(ctx context.Context) ([]model.ScanDefinition, error)
	UpdateScanDefinition(ctx context.Context, in model.ScanDefinition) (*model.ScanDefinition, error)
	DeleteScanDefinition(ctx context.Context, id string) error
	SetScanDefinitionEnabled(ctx context.Context, id string, enabled bool, nextRunAt *time.Time) error
	SetScanDefinitionNextRun(ctx context.Context, id string, nextRunAt *time.Time) error
	SetScanDefinitionLastRun(ctx context.Context, id string, at time.Time, status string) error
	ClaimDueScanDefinitions(ctx context.Context, now time.Time, nextFn func(schedule string, from time.Time) (time.Time, error), limit int) ([]model.ScanDefinition, error)

	// CreateScanForDefinition creates a `scans` row bound to a scan
	// definition. It returns a pending scan; the caller publishes the
	// directive after create succeeds.
	CreateScanForDefinition(ctx context.Context, in CreateScanForDefinitionInput) (*model.Scan, error)

	// Collection membership resolution — used for finding filters and
	// scheduler dispatch. Returns endpoint ids for scope=endpoint/finding
	// collections, asset ids for scope=asset.
	CollectionEndpointIDs(ctx context.Context, collectionID string) ([]string, error)
}

// --- Helper types passed through the store boundary ------------------

// UpsertAssetInput is what the discovery ingest path (P2) will hand to
// UpsertAsset. Kept narrow in P1 — P2 extends as ingest requires.
type UpsertAssetInput struct {
	TenantID     string
	PrimaryIP    string // "" → leave nil
	Hostname     string
	ResourceType string // empty defaults to 'host'
	Source       string // 'manual' | 'discovered'
	Environment  *string
	Fingerprint  json.RawMessage
}

// UpsertAssetEndpointInput mirrors UpsertAssetInput for asset_endpoints.
type UpsertAssetEndpointInput struct {
	AssetID         string
	Port            int
	Protocol        string // empty defaults to 'tcp'
	Service         string
	Version         string
	Technologies    json.RawMessage
	AllowlistStatus *string
}

// DiscoverySourceInput records provenance for a single discovery event
// (ADR 006 D9). ScanID is optional so manual asset creation can also
// log a source row later.
type DiscoverySourceInput struct {
	AssetID  string
	TargetID *string
	AgentID  *string
	ScanID   *string
}

// AgentAllowlistInput is the agent's reported scan-policy snapshot.
// Matches websocket.AllowlistSnapshotPayload field-for-field.
type AgentAllowlistInput struct {
	AgentID      string
	Hash         string
	Allow        []string
	Deny         []string
	RateLimitPPS int
}

// AgentAllowlistSnapshot is the stored row, returned for UI display
// and for rediscovery-free re-evaluation when the snapshot changes.
type AgentAllowlistSnapshot struct {
	AgentID      string    `json:"agent_id"`
	Hash         string    `json:"snapshot_hash"`
	Allow        []string  `json:"allow"`
	Deny         []string  `json:"deny"`
	RateLimitPPS int       `json:"rate_limit_pps"`
	ReportedAt   time.Time `json:"reported_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EndpointRow is the flattened (asset, endpoint) pair served to the
// P4 collection-preview path when scope=endpoint. Matches the shape
// the rule engine uses (asset + endpoint columns in one struct).
type EndpointRow struct {
	Asset    model.Asset
	Endpoint model.AssetEndpoint
}

// UpsertFindingInput is the write-through shape for the ADR 007 D2
// ingest paths (nuclei hits and compliance bundle results). Upsert key
// is (asset_endpoint_id, source_kind, source, source_id).
type UpsertFindingInput struct {
	TenantID        string
	AssetEndpointID string
	ScanID          *string
	SourceKind      string
	Source          string
	SourceID        string
	CVEID           *string
	Severity        *string
	Title           string
	Status          string
	Evidence        json.RawMessage
	Remediation     *string
}

// FindingFilter is the parsed query for ListFindings.
type FindingFilter struct {
	SourceKind      string
	Source          string
	Severity        string
	Status          string
	AssetEndpointID string
	CollectionID    string
	CVEID           string
	Since           *time.Time
	Until           *time.Time
	Limit           int
}

// CreateScanForDefinitionInput is the store-level input for materializing
// a scans row from a definition dispatch (scheduler, manual execute,
// or run_scan_definition rule action).
type CreateScanForDefinitionInput struct {
	TenantID         string
	ScanDefinitionID string
	AgentID          *string
	TargetID         *string
	AssetEndpointID  *string
	BundleID         *string
	ScanType         string
}

// AssetFilter is the parsed query for ListAssets.
type AssetFilter struct {
	Source      string
	Environment string
	Q           string
	Page        int
	PageSize    int
}

// --- Context plumbing ------------------------------------------------

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
