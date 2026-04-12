package store

import (
	"context"
	"encoding/json"

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

	// Scan Results
	CreateScanResults(ctx context.Context, scanID string, results []model.ScanResult) error
	GetScanResults(ctx context.Context, scanID string) ([]model.ScanResult, error)

	// Agents
	GetAgent(ctx context.Context, id string) (*model.Agent, error)
	GetAgentByID(ctx context.Context, id string) (*model.Agent, error) // not tenant-scoped, for WSS auth
	CreateAgent(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, string, error)
	UpdateAgentStatus(ctx context.Context, id string, status string) error
	RotateAgentKey(ctx context.Context, id string) (string, error)
	PromoteAgentKey(ctx context.Context, id string) error

	// Targets (non-tenant-scoped, for directive enrichment)
	GetTargetByID(ctx context.Context, id string) (*model.Target, error)

	// Bundles
	GetBundle(ctx context.Context, id string) (*model.Bundle, error)

	// Credentials
	GetCredentialsByTarget(ctx context.Context, targetID string) (json.RawMessage, error)

	// Health
	Ping(ctx context.Context) error
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
