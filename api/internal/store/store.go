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
	UpdateCredentialSourceConfig(ctx context.Context, id string, config json.RawMessage) error
	DeleteCredentialSource(ctx context.Context, id string) error
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
