package store

import (
	"context"

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
	UpdateAgentStatus(ctx context.Context, id string, status string) error

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
