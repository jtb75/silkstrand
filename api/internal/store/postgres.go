// Package store — PostgreSQL implementation of the Store interface.
//
// P1 asset-first refactor shape: legacy recon-pipeline methods (upsert
// discovered_assets, rules, channels, asset sets, one-shots, scan
// results) are gone along with the tables they read. The new surface
// is asset / endpoint / collection / finding / scan_definition; P1
// ships minimal working impls for collections + targets + the read
// side of scans/assets, and stub-returns ErrNotImplemented for the
// entities whose full impls land in P2+.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jtb75/silkstrand/api/internal/model"
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

func (s *PostgresStore) Close() error { return s.db.Close() }
func (s *PostgresStore) DB() *sql.DB  { return s.db }
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ======================================================================
// Targets (CIDR / network_range only — ADR 006 D8)
// ======================================================================

func (s *PostgresStore) ListTargets(ctx context.Context) ([]model.Target, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, credential_source_id, created_at, updated_at
		   FROM targets WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing targets: %w", err)
	}
	defer rows.Close()
	var targets []model.Target
	for rows.Next() {
		var t model.Target
		if err := rows.Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config,
			&t.Environment, &t.CredentialSourceID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *PostgresStore) GetTarget(ctx context.Context, id string) (*model.Target, error) {
	tenantID := TenantID(ctx)
	var t model.Target
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, credential_source_id, created_at, updated_at
		   FROM targets WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config,
			&t.Environment, &t.CredentialSourceID, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting target: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) GetTargetByID(ctx context.Context, id string) (*model.Target, error) {
	var t model.Target
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, credential_source_id, created_at, updated_at
		   FROM targets WHERE id = $1`, id).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config,
			&t.Environment, &t.CredentialSourceID, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting target by id: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) CreateTarget(ctx context.Context, req model.CreateTargetRequest) (*model.Target, error) {
	tenantID := TenantID(ctx)
	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}
	var envPtr *string
	if req.Environment != "" {
		e := req.Environment
		envPtr = &e
	}
	var t model.Target
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO targets (tenant_id, agent_id, type, identifier, config, environment)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, agent_id, type, identifier, config, environment, credential_source_id, created_at, updated_at`,
		tenantID, req.AgentID, req.Type, req.Identifier, cfg, envPtr).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config,
			&t.Environment, &t.CredentialSourceID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating target: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) UpdateTarget(ctx context.Context, id string, req model.UpdateTargetRequest) (*model.Target, error) {
	tenantID := TenantID(ctx)
	existing, err := s.GetTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if req.Type != nil {
		existing.Type = *req.Type
	}
	if req.Identifier != nil {
		existing.Identifier = *req.Identifier
	}
	if req.Config != nil {
		existing.Config = req.Config
	}
	if req.Environment != nil {
		existing.Environment = req.Environment
	}
	if req.AgentID != nil {
		existing.AgentID = req.AgentID
	}
	var t model.Target
	err = s.db.QueryRowContext(ctx,
		`UPDATE targets SET type = $1, identifier = $2, config = $3, environment = $4, agent_id = $5, updated_at = NOW()
		   WHERE id = $6 AND tenant_id = $7
		 RETURNING id, tenant_id, agent_id, type, identifier, config, environment, credential_source_id, created_at, updated_at`,
		existing.Type, existing.Identifier, existing.Config, existing.Environment, existing.AgentID, id, tenantID).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config,
			&t.Environment, &t.CredentialSourceID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("updating target: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) DeleteTarget(ctx context.Context, id string) error {
	tenantID := TenantID(ctx)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM targets WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting target: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ======================================================================
// Scans (execution history; ADR 007 D3). CreateScan is retained for the
// ad-hoc debug path (POST /api/v1/scans) — scope expansion for scan
// definitions lands in P3.
// ======================================================================

const scanCols = `id, tenant_id, scan_definition_id, agent_id, target_id, asset_endpoint_id, bundle_id, scan_type, status, error_message, started_at, completed_at, created_at`

func scanScan(sc *model.Scan, row interface{ Scan(...any) error }) error {
	return row.Scan(&sc.ID, &sc.TenantID, &sc.ScanDefinitionID, &sc.AgentID,
		&sc.TargetID, &sc.AssetEndpointID, &sc.BundleID, &sc.ScanType,
		&sc.Status, &sc.ErrorMessage, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
}

func (s *PostgresStore) ListScans(ctx context.Context) ([]model.Scan, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scanCols+` FROM scans WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing scans: %w", err)
	}
	defer rows.Close()
	var scans []model.Scan
	for rows.Next() {
		var sc model.Scan
		if err := scanScan(&sc, rows); err != nil {
			return nil, fmt.Errorf("scanning scan row: %w", err)
		}
		scans = append(scans, sc)
	}
	return scans, rows.Err()
}

func (s *PostgresStore) GetScan(ctx context.Context, id string) (*model.Scan, error) {
	tenantID := TenantID(ctx)
	var sc model.Scan
	row := s.db.QueryRowContext(ctx,
		`SELECT `+scanCols+` FROM scans WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err := scanScan(&sc, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scan: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) GetScanByID(ctx context.Context, id string) (*model.Scan, error) {
	var sc model.Scan
	row := s.db.QueryRowContext(ctx, `SELECT `+scanCols+` FROM scans WHERE id = $1`, id)
	if err := scanScan(&sc, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting scan by id: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) CreateScan(ctx context.Context, req model.CreateScanRequest) (*model.Scan, error) {
	tenantID := TenantID(ctx)
	target, err := s.GetTarget(ctx, req.TargetID)
	if err != nil {
		return nil, fmt.Errorf("looking up target: %w", err)
	}
	if target == nil {
		return nil, fmt.Errorf("target not found")
	}
	scanType := req.ScanType
	if scanType == "" {
		scanType = model.ScanTypeCompliance
	}
	var sc model.Scan
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO scans (tenant_id, agent_id, target_id, bundle_id, scan_type, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+scanCols,
		tenantID, target.AgentID, req.TargetID, req.BundleID, scanType, model.ScanStatusPending)
	if err := scanScan(&sc, row); err != nil {
		return nil, fmt.Errorf("creating scan: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) UpdateScanStatus(ctx context.Context, id, status string) error {
	var query string
	switch status {
	case model.ScanStatusRunning:
		query = `UPDATE scans SET status = $1, started_at = NOW() WHERE id = $2`
	case model.ScanStatusCompleted, model.ScanStatusFailed:
		query = `UPDATE scans SET status = $1, completed_at = NOW() WHERE id = $2`
	default:
		query = `UPDATE scans SET status = $1 WHERE id = $2`
	}
	if _, err := s.db.ExecContext(ctx, query, status, id); err != nil {
		return fmt.Errorf("updating scan status: %w", err)
	}
	return nil
}

func (s *PostgresStore) FailScan(ctx context.Context, id, reason string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scans SET status = $1, completed_at = NOW(), error_message = NULLIF($2, '') WHERE id = $3`,
		model.ScanStatusFailed, reason, id)
	if err != nil {
		return fmt.Errorf("failing scan: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteScan(ctx context.Context, id string) error {
	tenantID := TenantID(ctx)
	if tenantID == "" {
		return fmt.Errorf("tenant not set in context")
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM scans WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting scan: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PostgresStore) FailRunningScansForAgent(ctx context.Context, agentID string) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE scans SET status = $1, completed_at = NOW()
		   WHERE agent_id = $2 AND status IN ($3, $4)`,
		model.ScanStatusFailed, agentID, model.ScanStatusPending, model.ScanStatusRunning)
	if err != nil {
		return 0, fmt.Errorf("failing running scans for agent: %w", err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// ======================================================================
// Agents
// ======================================================================

func (s *PostgresStore) GetAgent(ctx context.Context, id string) (*model.Agent, error) {
	tenantID := TenantID(ctx)
	var a model.Agent
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, version, created_at
		   FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat, &a.Version, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting agent: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) GetAgentByID(ctx context.Context, id string) (*model.Agent, error) {
	var a model.Agent
	var keyHash sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, version, key_hash, next_key_hash, key_rotated_at, created_at
		   FROM agents WHERE id = $1`, id).
		Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat, &a.Version,
			&keyHash, &a.NextKeyHash, &a.KeyRotatedAt, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting agent by id: %w", err)
	}
	a.KeyHash = keyHash.String
	return &a, nil
}

func generateAgentKey() (raw string, hash string, err error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", "", fmt.Errorf("generating random key: %w", err)
	}
	rawKey := hex.EncodeToString(key)
	h := sha256.Sum256([]byte(rawKey))
	return rawKey, hex.EncodeToString(h[:]), nil
}

func (s *PostgresStore) CreateAgent(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, string, error) {
	rawKey, keyHash, err := generateAgentKey()
	if err != nil {
		return nil, "", err
	}
	var a model.Agent
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO agents (tenant_id, name, version, key_hash)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, name, status, last_heartbeat, version, key_hash, next_key_hash, key_rotated_at, created_at`,
		req.TenantID, req.Name, req.Version, keyHash).
		Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat, &a.Version,
			&a.KeyHash, &a.NextKeyHash, &a.KeyRotatedAt, &a.CreatedAt)
	if err != nil {
		return nil, "", fmt.Errorf("creating agent: %w", err)
	}
	return &a, rawKey, nil
}

func (s *PostgresStore) UpdateAgentStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status = $1, last_heartbeat = NOW() WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateAgentHeartbeat(ctx context.Context, id, version string) error {
	if version == "" {
		return s.UpdateAgentStatus(ctx, id, "connected")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status = 'connected', last_heartbeat = NOW(), version = $1 WHERE id = $2`,
		version, id)
	if err != nil {
		return fmt.Errorf("updating agent heartbeat: %w", err)
	}
	return nil
}

func (s *PostgresStore) RotateAgentKey(ctx context.Context, id string) (string, error) {
	rawKey, keyHash, err := generateAgentKey()
	if err != nil {
		return "", err
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE agents SET next_key_hash = $1 WHERE id = $2`, keyHash, id)
	if err != nil {
		return "", fmt.Errorf("rotating agent key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return "", fmt.Errorf("agent not found")
	}
	return rawKey, nil
}

func (s *PostgresStore) PromoteAgentKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE agents SET key_hash = next_key_hash, next_key_hash = NULL, key_rotated_at = NOW()
		   WHERE id = $1 AND next_key_hash IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("promoting agent key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("agent not found or no pending key rotation")
	}
	return nil
}

func (s *PostgresStore) ListAgents(ctx context.Context) ([]model.Agent, error) {
	tenantID := TenantID(ctx)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant not set in context")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, version, created_at
		   FROM agents WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()
	var out []model.Agent
	for rows.Next() {
		var a model.Agent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat, &a.Version, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListAllAgents(ctx context.Context) ([]model.Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, version, created_at
		   FROM agents ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing all agents: %w", err)
	}
	defer rows.Close()
	var agents []model.Agent
	for rows.Next() {
		var a model.Agent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat, &a.Version, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning agent: %w", err)
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *PostgresStore) DeleteAgent(ctx context.Context, id string) error {
	tenantID := TenantID(ctx)
	if tenantID == "" {
		return fmt.Errorf("tenant not set in context")
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM agents WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ======================================================================
// Install tokens
// ======================================================================

func (s *PostgresStore) CreateInstallToken(ctx context.Context, tenantID string, tokenHash []byte, expiresAt time.Time, createdBy string) error {
	var createdByArg interface{}
	if createdBy != "" {
		createdByArg = createdBy
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO install_tokens (token_hash, tenant_id, created_by, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		tokenHash, tenantID, createdByArg, expiresAt)
	if err != nil {
		return fmt.Errorf("creating install token: %w", err)
	}
	return nil
}

func (s *PostgresStore) ConsumeInstallToken(ctx context.Context, tokenHash []byte, agentID string) (string, error) {
	var tenantID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE install_tokens
		   SET used_at = NOW(), used_agent_id = $2
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		 RETURNING tenant_id`,
		tokenHash, agentID).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return "", sql.ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("consuming install token: %w", err)
	}
	return tenantID, nil
}

// ======================================================================
// Bundles
// ======================================================================

func (s *PostgresStore) GetBundle(ctx context.Context, id string) (*model.Bundle, error) {
	var b model.Bundle
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at
		   FROM bundles WHERE id = $1`, id).
		Scan(&b.ID, &b.TenantID, &b.Name, &b.Version, &b.Framework, &b.TargetType,
			&b.GCSPath, &b.Signature, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting bundle: %w", err)
	}
	return &b, nil
}

func (s *PostgresStore) ListBundlesForTenant(ctx context.Context, tenantID string) ([]model.Bundle, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at
		   FROM bundles WHERE tenant_id IS NULL OR tenant_id = $1 ORDER BY name, version`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing bundles: %w", err)
	}
	defer rows.Close()
	var out []model.Bundle
	for rows.Next() {
		var b model.Bundle
		if err := rows.Scan(&b.ID, &b.TenantID, &b.Name, &b.Version, &b.Framework, &b.TargetType,
			&b.GCSPath, &b.Signature, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning bundle: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpsertBundle(ctx context.Context, b model.Bundle) (*model.Bundle, error) {
	var out model.Bundle
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO bundles (id, tenant_id, name, version, framework, target_type, gcs_path, signature)
		 VALUES (COALESCE(NULLIF($1, '')::uuid, uuid_generate_v4()), $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO UPDATE SET
		   name = EXCLUDED.name, version = EXCLUDED.version, framework = EXCLUDED.framework,
		   target_type = EXCLUDED.target_type, gcs_path = EXCLUDED.gcs_path, signature = EXCLUDED.signature
		 RETURNING id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at`,
		b.ID, b.TenantID, b.Name, b.Version, b.Framework, b.TargetType, b.GCSPath, b.Signature).
		Scan(&out.ID, &out.TenantID, &out.Name, &out.Version, &out.Framework, &out.TargetType,
			&out.GCSPath, &out.Signature, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting bundle: %w", err)
	}
	return &out, nil
}

// ======================================================================
// Tenants (internal)
// ======================================================================

func (s *PostgresStore) CreateTenant(ctx context.Context, name string) (*model.Tenant, error) {
	var t model.Tenant
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO tenants (name) VALUES ($1)
		 RETURNING id, name, status, config, created_at`, name).
		Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating tenant: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) ListAllTenants(ctx context.Context) ([]model.Tenant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, status, config, created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing tenants: %w", err)
	}
	defer rows.Close()
	var tenants []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *PostgresStore) GetTenantByID(ctx context.Context, id string) (*model.Tenant, error) {
	var t model.Tenant
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, status, config, created_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.Status, &t.Config, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting tenant by id: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) UpdateTenantStatus(ctx context.Context, id, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tenants SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating tenant status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *PostgresStore) UpdateTenantConfig(ctx context.Context, id string, config json.RawMessage) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tenants SET config = $1 WHERE id = $2`, config, id)
	if err != nil {
		return fmt.Errorf("updating tenant config: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

func (s *PostgresStore) UpdateTenantName(ctx context.Context, id, name string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tenants SET name = $1 WHERE id = $2`, name, id)
	if err != nil {
		return fmt.Errorf("updating tenant name: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

// ======================================================================
// Stats
// ======================================================================

func (s *PostgresStore) GetDCStats(ctx context.Context) (*model.DCStats, error) {
	var stats model.DCStats
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenants WHERE status = $1`,
		model.TenantStatusActive).Scan(&stats.TenantCount); err != nil {
		return nil, fmt.Errorf("counting tenants: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&stats.AgentCount); err != nil {
		return nil, fmt.Errorf("counting agents: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`).Scan(&stats.ScanCount); err != nil {
		return nil, fmt.Errorf("counting scans: %w", err)
	}
	return &stats, nil
}

// ======================================================================
// Credential sources (ADR 004 C0)
// ======================================================================

func (s *PostgresStore) CreateCredentialSource(ctx context.Context, tenantID, srcType string, config json.RawMessage) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO credential_sources (tenant_id, type, config) VALUES ($1, $2, $3) RETURNING id`,
		tenantID, srcType, config).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating credential source: %w", err)
	}
	return id, nil
}

func (s *PostgresStore) GetCredentialSource(ctx context.Context, id string) (*model.CredentialSource, error) {
	var cs model.CredentialSource
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, config, created_at, updated_at FROM credential_sources WHERE id = $1`, id).
		Scan(&cs.ID, &cs.TenantID, &cs.Type, &cs.Config, &cs.CreatedAt, &cs.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting credential source: %w", err)
	}
	return &cs, nil
}

func (s *PostgresStore) GetCredentialSourceByTarget(ctx context.Context, targetID string) (*model.CredentialSource, error) {
	var cs model.CredentialSource
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.id, cs.tenant_id, cs.type, cs.config, cs.created_at, cs.updated_at
		   FROM credential_sources cs JOIN targets t ON t.credential_source_id = cs.id
		  WHERE t.id = $1`, targetID).
		Scan(&cs.ID, &cs.TenantID, &cs.Type, &cs.Config, &cs.CreatedAt, &cs.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting credential source by target: %w", err)
	}
	return &cs, nil
}

func (s *PostgresStore) UpdateCredentialSourceConfig(ctx context.Context, id string, config json.RawMessage) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE credential_sources SET config = $1, updated_at = NOW() WHERE id = $2`, config, id)
	if err != nil {
		return fmt.Errorf("updating credential source: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PostgresStore) DeleteCredentialSource(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credential_sources WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting credential source: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetTargetCredentialSource(ctx context.Context, targetID, sourceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET credential_source_id = $1, updated_at = NOW() WHERE id = $2`, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("setting target credential source: %w", err)
	}
	return nil
}

func (s *PostgresStore) ClearTargetCredentialSource(ctx context.Context, targetID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET credential_source_id = NULL, updated_at = NOW() WHERE id = $1`, targetID)
	if err != nil {
		return fmt.Errorf("clearing target credential source: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpsertStaticCredentialSource(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) error {
	cfgJSON, err := json.Marshal(model.StaticCredentialConfig{
		Type:          credType,
		EncryptedData: base64.StdEncoding.EncodeToString(encryptedData),
	})
	if err != nil {
		return fmt.Errorf("marshalling static credential config: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID, existingType sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT cs.id, cs.type FROM targets t
		   LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		  WHERE t.id = $1`, targetID).Scan(&existingID, &existingType)
	if err != nil {
		return fmt.Errorf("looking up existing source: %w", err)
	}
	switch {
	case existingID.Valid && existingType.String == model.CredentialSourceTypeStatic:
		if _, err := tx.ExecContext(ctx,
			`UPDATE credential_sources SET config = $1, updated_at = NOW() WHERE id = $2`,
			cfgJSON, existingID.String); err != nil {
			return fmt.Errorf("updating static credential source: %w", err)
		}
	case existingID.Valid && existingType.String != model.CredentialSourceTypeStatic:
		return fmt.Errorf("target %s has non-static credential source; cannot overwrite", targetID)
	default:
		var newID string
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO credential_sources (tenant_id, type, config) VALUES ($1, $2, $3) RETURNING id`,
			tenantID, model.CredentialSourceTypeStatic, cfgJSON).Scan(&newID); err != nil {
			return fmt.Errorf("inserting static credential source: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE targets SET credential_source_id = $1, updated_at = NOW() WHERE id = $2`,
			newID, targetID); err != nil {
			return fmt.Errorf("linking target to credential source: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetStaticCredentialForTarget(ctx context.Context, targetID string) ([]byte, string, error) {
	var srcType, cfgRaw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.type, cs.config::text FROM targets t
		   LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		  WHERE t.id = $1`, targetID).Scan(&srcType, &cfgRaw)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", fmt.Errorf("looking up credential source: %w", err)
	}
	if !srcType.Valid || srcType.String != model.CredentialSourceTypeStatic || !cfgRaw.Valid {
		return nil, "", nil
	}
	var cfg model.StaticCredentialConfig
	if err := json.Unmarshal([]byte(cfgRaw.String), &cfg); err != nil {
		return nil, "", fmt.Errorf("decoding static credential config: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(cfg.EncryptedData)
	if err != nil {
		return nil, "", fmt.Errorf("decoding static credential data: %w", err)
	}
	return data, cfg.Type, nil
}

func (s *PostgresStore) HasCredentialForTarget(ctx context.Context, targetID string) (bool, string, error) {
	var srcType, cfgRaw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.type, cs.config::text FROM targets t
		   LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		  WHERE t.id = $1`, targetID).Scan(&srcType, &cfgRaw)
	if err != nil && err != sql.ErrNoRows {
		return false, "", fmt.Errorf("checking credential source: %w", err)
	}
	if !srcType.Valid || srcType.String != model.CredentialSourceTypeStatic || !cfgRaw.Valid {
		return false, "", nil
	}
	var cfg model.StaticCredentialConfig
	if err := json.Unmarshal([]byte(cfgRaw.String), &cfg); err != nil {
		return false, "", fmt.Errorf("decoding static credential config: %w", err)
	}
	return true, cfg.Type, nil
}

func (s *PostgresStore) DeleteCredentialForTarget(ctx context.Context, tenantID, targetID string) error {
	_ = tenantID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceID, sourceType sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT cs.id, cs.type FROM targets t
		   LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		  WHERE t.id = $1`, targetID).Scan(&sourceID, &sourceType); err != nil {
		return fmt.Errorf("looking up credential source: %w", err)
	}
	if !sourceID.Valid || sourceType.String != model.CredentialSourceTypeStatic {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE targets SET credential_source_id = NULL, updated_at = NOW() WHERE id = $1`,
		targetID); err != nil {
		return fmt.Errorf("clearing target credential source: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM credential_sources WHERE id = $1`, sourceID.String); err != nil {
		return fmt.Errorf("deleting credential source: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ======================================================================
// Assets + endpoints (ADR 006 D2) — minimal working read impls; write
// path is wired for P2 ingest to call.
// ======================================================================

func (s *PostgresStore) UpsertAsset(ctx context.Context, in UpsertAssetInput) (*model.Asset, error) {
	if in.ResourceType == "" {
		in.ResourceType = model.ResourceTypeHost
	}
	fp := in.Fingerprint
	if len(fp) == 0 {
		fp = json.RawMessage(`{}`)
	}
	var primaryIP *string
	if in.PrimaryIP != "" {
		p := in.PrimaryIP
		primaryIP = &p
	}
	var hostname *string
	if in.Hostname != "" {
		h := in.Hostname
		hostname = &h
	}
	// Postgres partial unique index (tenant_id, primary_ip) WHERE primary_ip IS NOT NULL
	// handles the upsert shape for real hosts. For IP-less assets we always insert.
	var a model.Asset
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO assets (tenant_id, primary_ip, hostname, fingerprint, resource_type, source, environment)
		 VALUES ($1, $2::inet, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, primary_ip) WHERE primary_ip IS NOT NULL DO UPDATE SET
		   hostname = COALESCE(EXCLUDED.hostname, assets.hostname),
		   fingerprint = assets.fingerprint || EXCLUDED.fingerprint,
		   last_seen = NOW()
		 RETURNING id, tenant_id, host(primary_ip), hostname, fingerprint, resource_type, source, environment, first_seen, last_seen, created_at`,
		in.TenantID, primaryIP, hostname, fp, in.ResourceType, in.Source, in.Environment).
		Scan(&a.ID, &a.TenantID, &a.PrimaryIP, &a.Hostname, &a.Fingerprint, &a.ResourceType,
			&a.Source, &a.Environment, &a.FirstSeen, &a.LastSeen, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting asset: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) UpsertAssetEndpoint(ctx context.Context, in UpsertAssetEndpointInput) (*model.AssetEndpoint, error) {
	if in.Protocol == "" {
		in.Protocol = "tcp"
	}
	tech := in.Technologies
	if len(tech) == 0 {
		tech = json.RawMessage(`[]`)
	}
	var svc, ver *string
	if in.Service != "" {
		v := in.Service
		svc = &v
	}
	if in.Version != "" {
		v := in.Version
		ver = &v
	}
	var ae model.AssetEndpoint
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO asset_endpoints (asset_id, port, protocol, service, version, technologies, allowlist_status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (asset_id, port, protocol) DO UPDATE SET
		   service = COALESCE(EXCLUDED.service, asset_endpoints.service),
		   version = COALESCE(EXCLUDED.version, asset_endpoints.version),
		   technologies = EXCLUDED.technologies,
		   allowlist_status = COALESCE(EXCLUDED.allowlist_status, asset_endpoints.allowlist_status),
		   last_seen = NOW(),
		   updated_at = NOW()
		 RETURNING id, asset_id, port, protocol, service, version, technologies,
		           compliance_status, allowlist_status, allowlist_checked_at,
		           last_scan_id, missed_scan_count, metadata,
		           first_seen, last_seen, created_at, updated_at`,
		in.AssetID, in.Port, in.Protocol, svc, ver, tech, in.AllowlistStatus).
		Scan(&ae.ID, &ae.AssetID, &ae.Port, &ae.Protocol, &ae.Service, &ae.Version,
			&ae.Technologies, &ae.ComplianceStatus, &ae.AllowlistStatus, &ae.AllowlistCheckedAt,
			&ae.LastScanID, &ae.MissedScanCount, &ae.Metadata,
			&ae.FirstSeen, &ae.LastSeen, &ae.CreatedAt, &ae.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting asset_endpoint: %w", err)
	}
	return &ae, nil
}

func (s *PostgresStore) GetAssetByID(ctx context.Context, id string) (*model.Asset, error) {
	var a model.Asset
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, host(primary_ip), hostname, fingerprint, resource_type, source, environment,
		        first_seen, last_seen, created_at
		   FROM assets WHERE id = $1`, id).
		Scan(&a.ID, &a.TenantID, &a.PrimaryIP, &a.Hostname, &a.Fingerprint, &a.ResourceType,
			&a.Source, &a.Environment, &a.FirstSeen, &a.LastSeen, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting asset: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) ListAssets(ctx context.Context, filter AssetFilter) ([]model.Asset, int, error) {
	tenantID := TenantID(ctx)
	if filter.PageSize <= 0 || filter.PageSize > 500 {
		filter.PageSize = 50
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	offset := (filter.Page - 1) * filter.PageSize
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, host(primary_ip), hostname, fingerprint, resource_type, source, environment,
		        first_seen, last_seen, created_at
		   FROM assets
		  WHERE tenant_id = $1
		    AND ($2 = '' OR source = $2)
		  ORDER BY last_seen DESC
		  LIMIT $3 OFFSET $4`,
		tenantID, filter.Source, filter.PageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing assets: %w", err)
	}
	defer rows.Close()
	var items []model.Asset
	for rows.Next() {
		var a model.Asset
		if err := rows.Scan(&a.ID, &a.TenantID, &a.PrimaryIP, &a.Hostname, &a.Fingerprint,
			&a.ResourceType, &a.Source, &a.Environment, &a.FirstSeen, &a.LastSeen, &a.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scanning asset: %w", err)
		}
		items = append(items, a)
	}
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM assets WHERE tenant_id = $1 AND ($2 = '' OR source = $2)`,
		tenantID, filter.Source).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting assets: %w", err)
	}
	return items, total, nil
}

// ======================================================================
// Collections (ADR 006 D5)
// ======================================================================

const collectionCols = `id, tenant_id, name, description, scope, predicate, is_dashboard_widget, widget_kind, created_at, updated_at, created_by`

func scanCollection(c *model.Collection, row interface{ Scan(...any) error }) error {
	return row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.Scope, &c.Predicate,
		&c.IsDashboardWidget, &c.WidgetKind, &c.CreatedAt, &c.UpdatedAt, &c.CreatedBy)
}

func (s *PostgresStore) CreateCollection(ctx context.Context, c model.Collection) (*model.Collection, error) {
	tenantID := TenantID(ctx)
	if c.Scope == "" {
		c.Scope = model.CollectionScopeEndpoint
	}
	if len(c.Predicate) == 0 {
		c.Predicate = json.RawMessage(`{}`)
	}
	var out model.Collection
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO collections (tenant_id, name, description, scope, predicate, is_dashboard_widget, widget_kind, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+collectionCols,
		tenantID, c.Name, c.Description, c.Scope, c.Predicate, c.IsDashboardWidget, c.WidgetKind, c.CreatedBy)
	if err := scanCollection(&out, row); err != nil {
		return nil, fmt.Errorf("creating collection: %w", err)
	}
	return &out, nil
}

func (s *PostgresStore) GetCollection(ctx context.Context, id string) (*model.Collection, error) {
	tenantID := TenantID(ctx)
	var c model.Collection
	row := s.db.QueryRowContext(ctx,
		`SELECT `+collectionCols+` FROM collections WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err := scanCollection(&c, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting collection: %w", err)
	}
	return &c, nil
}

func (s *PostgresStore) ListCollections(ctx context.Context) ([]model.Collection, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+collectionCols+` FROM collections WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing collections: %w", err)
	}
	defer rows.Close()
	var out []model.Collection
	for rows.Next() {
		var c model.Collection
		if err := scanCollection(&c, rows); err != nil {
			return nil, fmt.Errorf("scanning collection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateCollection(ctx context.Context, c model.Collection) (*model.Collection, error) {
	tenantID := TenantID(ctx)
	var out model.Collection
	row := s.db.QueryRowContext(ctx,
		`UPDATE collections SET
		   name = $1, description = $2, scope = $3, predicate = $4,
		   is_dashboard_widget = $5, widget_kind = $6, updated_at = NOW()
		 WHERE id = $7 AND tenant_id = $8
		 RETURNING `+collectionCols,
		c.Name, c.Description, c.Scope, c.Predicate, c.IsDashboardWidget, c.WidgetKind, c.ID, tenantID)
	if err := scanCollection(&out, row); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("updating collection: %w", err)
	}
	return &out, nil
}

func (s *PostgresStore) DeleteCollection(ctx context.Context, id string) error {
	tenantID := TenantID(ctx)
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM collections WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return fmt.Errorf("deleting collection: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ======================================================================
// P2 ingest surface (ADR 006 D4, D7, D9)
// ======================================================================

func (s *PostgresStore) AppendAssetEvents(ctx context.Context, events []model.AssetEvent) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO asset_events (tenant_id, asset_id, scan_id, event_type, severity, payload, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()))`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()
	for _, e := range events {
		payload := e.Payload
		if len(payload) == 0 {
			payload = json.RawMessage(`{}`)
		}
		var occurred interface{}
		if !e.OccurredAt.IsZero() {
			occurred = e.OccurredAt
		}
		if _, err := stmt.ExecContext(ctx,
			e.TenantID, e.AssetID, e.ScanID, e.EventType, e.Severity, payload, occurred); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *PostgresStore) RecordDiscoverySource(ctx context.Context, in DiscoverySourceInput) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO asset_discovery_sources (asset_id, target_id, agent_id, scan_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (asset_id, discovered_at) DO NOTHING`,
		in.AssetID, in.TargetID, in.AgentID, in.ScanID)
	if err != nil {
		return fmt.Errorf("recording discovery source: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListEndpointsForAsset(ctx context.Context, assetID string) ([]model.AssetEndpoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, asset_id, port, protocol, service, version, technologies,
		        compliance_status, allowlist_status, allowlist_checked_at,
		        last_scan_id, missed_scan_count, metadata,
		        first_seen, last_seen, created_at, updated_at
		   FROM asset_endpoints WHERE asset_id = $1`, assetID)
	if err != nil {
		return nil, fmt.Errorf("listing endpoints: %w", err)
	}
	defer rows.Close()
	var out []model.AssetEndpoint
	for rows.Next() {
		var e model.AssetEndpoint
		if err := rows.Scan(&e.ID, &e.AssetID, &e.Port, &e.Protocol, &e.Service, &e.Version,
			&e.Technologies, &e.ComplianceStatus, &e.AllowlistStatus, &e.AllowlistCheckedAt,
			&e.LastScanID, &e.MissedScanCount, &e.Metadata,
			&e.FirstSeen, &e.LastSeen, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning endpoint: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateEndpointAllowlistStatus(ctx context.Context, endpointID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE asset_endpoints
		    SET allowlist_status = $1,
		        allowlist_checked_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $2`, status, endpointID)
	if err != nil {
		return fmt.Errorf("updating endpoint allowlist status: %w", err)
	}
	return nil
}

// ======================================================================
// Correlation rules (ADR 006 D6) — load side only in P2; CRUD in P2+.
// ======================================================================

func (s *PostgresStore) ListActiveRulesForTrigger(ctx context.Context, tenantID, trigger string) ([]model.CorrelationRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, version, enabled, trigger, event_type_filter, body, created_at, created_by
		   FROM correlation_rules
		  WHERE tenant_id = $1 AND enabled = TRUE AND trigger = $2`,
		tenantID, trigger)
	if err != nil {
		return nil, fmt.Errorf("listing rules: %w", err)
	}
	defer rows.Close()
	var out []model.CorrelationRule
	for rows.Next() {
		var r model.CorrelationRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Version, &r.Enabled,
			&r.Trigger, &r.EventTypeFilter, &r.Body, &r.CreatedAt, &r.CreatedBy); err != nil {
			return nil, fmt.Errorf("scanning rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ======================================================================
// Agent allowlists (migration 018)
// ======================================================================

func (s *PostgresStore) UpsertAgentAllowlist(ctx context.Context, in AgentAllowlistInput) (bool, error) {
	allow, _ := json.Marshal(in.Allow)
	deny, _ := json.Marshal(in.Deny)
	if len(in.Allow) == 0 {
		allow = json.RawMessage(`[]`)
	}
	if len(in.Deny) == 0 {
		deny = json.RawMessage(`[]`)
	}
	var existingHash sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT snapshot_hash FROM agent_allowlists WHERE agent_id = $1`, in.AgentID).
		Scan(&existingHash)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("loading existing allowlist: %w", err)
	}
	changed := !existingHash.Valid || existingHash.String != in.Hash
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO agent_allowlists (agent_id, snapshot_hash, allow, deny, rate_limit_pps, reported_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		 ON CONFLICT (agent_id) DO UPDATE SET
		   snapshot_hash = EXCLUDED.snapshot_hash,
		   allow = EXCLUDED.allow,
		   deny = EXCLUDED.deny,
		   rate_limit_pps = EXCLUDED.rate_limit_pps,
		   reported_at = NOW(),
		   updated_at = NOW()`,
		in.AgentID, in.Hash, allow, deny, in.RateLimitPPS)
	if err != nil {
		return false, fmt.Errorf("upserting allowlist: %w", err)
	}
	return changed, nil
}

func (s *PostgresStore) GetAgentAllowlist(ctx context.Context, agentID string) (*AgentAllowlistSnapshot, error) {
	var snap AgentAllowlistSnapshot
	var allowJSON, denyJSON []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT agent_id, snapshot_hash, allow, deny, rate_limit_pps, reported_at, updated_at
		   FROM agent_allowlists WHERE agent_id = $1`, agentID).
		Scan(&snap.AgentID, &snap.Hash, &allowJSON, &denyJSON, &snap.RateLimitPPS, &snap.ReportedAt, &snap.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting allowlist: %w", err)
	}
	_ = json.Unmarshal(allowJSON, &snap.Allow)
	_ = json.Unmarshal(denyJSON, &snap.Deny)
	return &snap, nil
}

// ======================================================================
// Notification deliveries (ADR 006 P6)
// ======================================================================

func (s *PostgresStore) InsertNotificationDelivery(ctx context.Context, d model.NotificationDelivery) error {
	payload := d.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_deliveries
		   (tenant_id, channel_source_id, rule_id, event_id, severity, status,
		    attempt, response_code, error, payload, dispatched_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
		d.TenantID, d.ChannelSourceID, d.RuleID, d.EventID, d.Severity, d.Status,
		d.Attempt, d.ResponseCode, d.Error, payload)
	if err != nil {
		return fmt.Errorf("inserting notification delivery: %w", err)
	}
	return nil
}
