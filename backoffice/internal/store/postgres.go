package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// --- Data Centers ---

func (s *PostgresStore) ListDataCenters(ctx context.Context) ([]model.DataCenter, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, region, environment, api_url, api_key_encrypted, status, last_health_check, last_health_status, created_at, updated_at
		 FROM data_centers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing data centers: %w", err)
	}
	defer rows.Close()

	var dcs []model.DataCenter
	for rows.Next() {
		var dc model.DataCenter
		if err := rows.Scan(&dc.ID, &dc.Name, &dc.Region, &dc.Environment, &dc.APIURL, &dc.APIKeyEncrypted,
			&dc.Status, &dc.LastHealthCheck, &dc.LastHealthStatus, &dc.CreatedAt, &dc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning data center: %w", err)
		}
		dcs = append(dcs, dc)
	}
	return dcs, rows.Err()
}

func (s *PostgresStore) GetDataCenter(ctx context.Context, id string) (*model.DataCenter, error) {
	var dc model.DataCenter
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, region, environment, api_url, api_key_encrypted, status, last_health_check, last_health_status, created_at, updated_at
		 FROM data_centers WHERE id = $1`, id).
		Scan(&dc.ID, &dc.Name, &dc.Region, &dc.Environment, &dc.APIURL, &dc.APIKeyEncrypted,
			&dc.Status, &dc.LastHealthCheck, &dc.LastHealthStatus, &dc.CreatedAt, &dc.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting data center: %w", err)
	}
	return &dc, nil
}

func (s *PostgresStore) CreateDataCenter(ctx context.Context, dc model.DataCenter) (*model.DataCenter, error) {
	env := dc.Environment
	if env == "" {
		env = model.DCEnvStage
	}
	var created model.DataCenter
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO data_centers (name, region, environment, api_url, api_key_encrypted, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, region, environment, api_url, api_key_encrypted, status, last_health_check, last_health_status, created_at, updated_at`,
		dc.Name, dc.Region, env, dc.APIURL, dc.APIKeyEncrypted, model.DCStatusActive).
		Scan(&created.ID, &created.Name, &created.Region, &created.Environment, &created.APIURL, &created.APIKeyEncrypted,
			&created.Status, &created.LastHealthCheck, &created.LastHealthStatus, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating data center: %w", err)
	}
	return &created, nil
}

func (s *PostgresStore) UpdateDataCenter(ctx context.Context, id string, dc model.DataCenter) (*model.DataCenter, error) {
	var updated model.DataCenter
	err := s.db.QueryRowContext(ctx,
		`UPDATE data_centers SET name = $1, region = $2, environment = $3, api_url = $4, api_key_encrypted = $5, status = $6, updated_at = NOW()
		 WHERE id = $7
		 RETURNING id, name, region, environment, api_url, api_key_encrypted, status, last_health_check, last_health_status, created_at, updated_at`,
		dc.Name, dc.Region, dc.Environment, dc.APIURL, dc.APIKeyEncrypted, dc.Status, id).
		Scan(&updated.ID, &updated.Name, &updated.Region, &updated.Environment, &updated.APIURL, &updated.APIKeyEncrypted,
			&updated.Status, &updated.LastHealthCheck, &updated.LastHealthStatus, &updated.CreatedAt, &updated.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("updating data center: %w", err)
	}
	return &updated, nil
}

func (s *PostgresStore) DeleteDataCenter(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE data_centers SET status = $1, updated_at = NOW() WHERE id = $2`, model.DCStatusInactive, id)
	if err != nil {
		return fmt.Errorf("soft-deleting data center: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PostgresStore) UpdateDataCenterHealth(ctx context.Context, id string, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE data_centers SET last_health_check = NOW(), last_health_status = $1, updated_at = NOW() WHERE id = $2`,
		status, id)
	if err != nil {
		return fmt.Errorf("updating data center health: %w", err)
	}
	return nil
}

// --- Tenants ---

func (s *PostgresStore) ListTenants(ctx context.Context) ([]model.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, dc_tenant_id, data_center_id, name, status, config, provisioning_status, clerk_org_id, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()

	var tenants []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(&t.ID, &t.DCTenantID, &t.DataCenterID, &t.Name, &t.Status, &t.Config,
			&t.ProvisioningStatus, &t.ClerkOrgID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *PostgresStore) GetTenant(ctx context.Context, id string) (*model.Tenant, error) {
	var t model.Tenant
	err := s.db.QueryRowContext(ctx,
		`SELECT id, dc_tenant_id, data_center_id, name, status, config, provisioning_status, clerk_org_id, created_at, updated_at
		 FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.DCTenantID, &t.DataCenterID, &t.Name, &t.Status, &t.Config,
			&t.ProvisioningStatus, &t.ClerkOrgID, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting tenant: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) CreateTenant(ctx context.Context, t model.Tenant) (*model.Tenant, error) {
	cfg := t.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	var created model.Tenant
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO tenants (data_center_id, name, config, provisioning_status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, dc_tenant_id, data_center_id, name, status, config, provisioning_status, clerk_org_id, created_at, updated_at`,
		t.DataCenterID, t.Name, cfg, model.ProvisioningPending).
		Scan(&created.ID, &created.DCTenantID, &created.DataCenterID, &created.Name, &created.Status, &created.Config,
			&created.ProvisioningStatus, &created.ClerkOrgID, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating tenant: %w", err)
	}
	return &created, nil
}

func (s *PostgresStore) UpdateTenant(ctx context.Context, id string, name *string, config []byte) (*model.Tenant, error) {
	existing, err := s.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	if name != nil {
		existing.Name = *name
	}
	if config != nil {
		existing.Config = config
	}

	var updated model.Tenant
	err = s.db.QueryRowContext(ctx,
		`UPDATE tenants SET name = $1, config = $2, updated_at = NOW()
		 WHERE id = $3
		 RETURNING id, dc_tenant_id, data_center_id, name, status, config, provisioning_status, clerk_org_id, created_at, updated_at`,
		existing.Name, existing.Config, id).
		Scan(&updated.ID, &updated.DCTenantID, &updated.DataCenterID, &updated.Name, &updated.Status, &updated.Config,
			&updated.ProvisioningStatus, &updated.ClerkOrgID, &updated.CreatedAt, &updated.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("updating tenant: %w", err)
	}
	return &updated, nil
}

func (s *PostgresStore) UpdateTenantStatus(ctx context.Context, id string, status string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating tenant status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *PostgresStore) UpdateTenantClerkOrg(ctx context.Context, id string, clerkOrgID *string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET clerk_org_id = $1, updated_at = NOW() WHERE id = $2`, clerkOrgID, id)
	if err != nil {
		return fmt.Errorf("updating tenant clerk_org_id: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *PostgresStore) UpdateTenantProvisioning(ctx context.Context, id string, provStatus string, dcTenantID *string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE tenants SET provisioning_status = $1, dc_tenant_id = $2, updated_at = NOW() WHERE id = $3`,
		provStatus, dcTenantID, id)
	if err != nil {
		return fmt.Errorf("updating tenant provisioning: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *PostgresStore) ListTenantsByDataCenter(ctx context.Context, dcID string) ([]model.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, dc_tenant_id, data_center_id, name, status, config, provisioning_status, clerk_org_id, created_at, updated_at
		 FROM tenants WHERE data_center_id = $1 ORDER BY created_at DESC`, dcID)
	if err != nil {
		return nil, fmt.Errorf("listing tenants by data center: %w", err)
	}
	defer rows.Close()

	var tenants []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(&t.ID, &t.DCTenantID, &t.DataCenterID, &t.Name, &t.Status, &t.Config,
			&t.ProvisioningStatus, &t.ClerkOrgID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// --- Admin Users ---

func (s *PostgresStore) GetAdminByEmail(ctx context.Context, email string) (*model.AdminUser, error) {
	var a model.AdminUser
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, created_at FROM admin_users WHERE email = $1`, email).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Role, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting admin by email: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) CreateAdmin(ctx context.Context, email, passwordHash, role string) (*model.AdminUser, error) {
	var a model.AdminUser
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO admin_users (email, password_hash, role) VALUES ($1, $2, $3)
		 RETURNING id, email, password_hash, role, created_at`,
		email, passwordHash, role).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Role, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating admin: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting admins: %w", err)
	}
	return count, nil
}
