package store

import (
	"context"

	"github.com/jtb75/silkstrand/backoffice/internal/model"
)

// Store defines the data access interface for the backoffice.
type Store interface {
	// Data Centers
	ListDataCenters(ctx context.Context) ([]model.DataCenter, error)
	GetDataCenter(ctx context.Context, id string) (*model.DataCenter, error)
	CreateDataCenter(ctx context.Context, dc model.DataCenter) (*model.DataCenter, error)
	UpdateDataCenter(ctx context.Context, id string, dc model.DataCenter) (*model.DataCenter, error)
	DeleteDataCenter(ctx context.Context, id string) error
	UpdateDataCenterHealth(ctx context.Context, id string, status string) error

	// Tenants
	ListTenants(ctx context.Context) ([]model.Tenant, error)
	GetTenant(ctx context.Context, id string) (*model.Tenant, error)
	CreateTenant(ctx context.Context, t model.Tenant) (*model.Tenant, error)
	UpdateTenant(ctx context.Context, id string, name *string, config []byte) (*model.Tenant, error)
	UpdateTenantStatus(ctx context.Context, id string, status string) error
	UpdateTenantProvisioning(ctx context.Context, id string, provStatus string, dcTenantID *string) error
	UpdateTenantClerkOrg(ctx context.Context, id string, clerkOrgID *string) error
	ListTenantsByDataCenter(ctx context.Context, dcID string) ([]model.Tenant, error)

	// Admin Users
	GetAdminByEmail(ctx context.Context, email string) (*model.AdminUser, error)
	CreateAdmin(ctx context.Context, email, passwordHash, role string) (*model.AdminUser, error)
	CountAdmins(ctx context.Context) (int, error)

	// Health
	Ping(ctx context.Context) error
}
