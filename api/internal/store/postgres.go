package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// --- Targets ---

func (s *PostgresStore) ListTargets(ctx context.Context) ([]model.Target, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, created_at, updated_at
		 FROM targets WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing targets: %w", err)
	}
	defer rows.Close()

	var targets []model.Target
	for rows.Next() {
		var t model.Target
		if err := rows.Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config, &t.Environment, &t.CreatedAt, &t.UpdatedAt); err != nil {
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
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, created_at, updated_at
		 FROM targets WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config, &t.Environment, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting target: %w", err)
	}
	return &t, nil
}

func (s *PostgresStore) CreateTarget(ctx context.Context, req model.CreateTargetRequest) (*model.Target, error) {
	tenantID := TenantID(ctx)
	cfg := req.Config
	if cfg == nil {
		cfg = json.RawMessage(`{}`)
	}

	var t model.Target
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO targets (tenant_id, agent_id, type, identifier, config, environment)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, agent_id, type, identifier, config, environment, created_at, updated_at`,
		tenantID, req.AgentID, req.Type, req.Identifier, cfg, req.Environment).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config, &t.Environment, &t.CreatedAt, &t.UpdatedAt)
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
		 RETURNING id, tenant_id, agent_id, type, identifier, config, environment, created_at, updated_at`,
		existing.Type, existing.Identifier, existing.Config, existing.Environment, existing.AgentID, id, tenantID).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config, &t.Environment, &t.CreatedAt, &t.UpdatedAt)
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

// --- Scans ---

func (s *PostgresStore) ListScans(ctx context.Context) ([]model.Scan, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, agent_id, target_id, bundle_id, status, started_at, completed_at, created_at
		 FROM scans WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing scans: %w", err)
	}
	defer rows.Close()

	var scans []model.Scan
	for rows.Next() {
		var sc model.Scan
		if err := rows.Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning scan row: %w", err)
		}
		scans = append(scans, sc)
	}
	return scans, rows.Err()
}

func (s *PostgresStore) GetScan(ctx context.Context, id string) (*model.Scan, error) {
	tenantID := TenantID(ctx)
	var sc model.Scan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, target_id, bundle_id, status, started_at, completed_at, created_at
		 FROM scans WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting scan: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) CreateScan(ctx context.Context, req model.CreateScanRequest) (*model.Scan, error) {
	tenantID := TenantID(ctx)

	// Look up the target to find the assigned agent
	target, err := s.GetTarget(ctx, req.TargetID)
	if err != nil {
		return nil, fmt.Errorf("looking up target: %w", err)
	}
	if target == nil {
		return nil, fmt.Errorf("target not found")
	}

	var sc model.Scan
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO scans (tenant_id, agent_id, target_id, bundle_id, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, agent_id, target_id, bundle_id, status, started_at, completed_at, created_at`,
		tenantID, target.AgentID, req.TargetID, req.BundleID, model.ScanStatusPending).
		Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating scan: %w", err)
	}
	return &sc, nil
}

func (s *PostgresStore) UpdateScanStatus(ctx context.Context, id string, status string) error {
	var query string
	switch status {
	case model.ScanStatusRunning:
		query = `UPDATE scans SET status = $1, started_at = NOW() WHERE id = $2`
	case model.ScanStatusCompleted, model.ScanStatusFailed:
		query = `UPDATE scans SET status = $1, completed_at = NOW() WHERE id = $2`
	default:
		query = `UPDATE scans SET status = $1 WHERE id = $2`
	}

	_, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating scan status: %w", err)
	}
	return nil
}

// --- Scan Results ---

func (s *PostgresStore) CreateScanResults(ctx context.Context, scanID string, results []model.ScanResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO scan_results (scan_id, control_id, title, status, severity, evidence, remediation)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, r := range results {
		evidence := r.Evidence
		if evidence == nil {
			evidence = json.RawMessage(`{}`)
		}
		if _, err := stmt.ExecContext(ctx, scanID, r.ControlID, r.Title, r.Status, r.Severity, evidence, r.Remediation); err != nil {
			return fmt.Errorf("inserting scan result: %w", err)
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetScanResults(ctx context.Context, scanID string) ([]model.ScanResult, error) {
	tenantID := TenantID(ctx)
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.scan_id, r.control_id, r.title, r.status, r.severity, r.evidence, r.remediation, r.created_at
		 FROM scan_results r JOIN scans s ON r.scan_id = s.id
		 WHERE r.scan_id = $1 AND s.tenant_id = $2
		 ORDER BY r.control_id`, scanID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting scan results: %w", err)
	}
	defer rows.Close()

	var results []model.ScanResult
	for rows.Next() {
		var r model.ScanResult
		if err := rows.Scan(&r.ID, &r.ScanID, &r.ControlID, &r.Title, &r.Status, &r.Severity, &r.Evidence, &r.Remediation, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning result row: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// --- Agents ---

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

func (s *PostgresStore) UpdateAgentStatus(ctx context.Context, id string, status string) error {
	query := `UPDATE agents SET status = $1, last_heartbeat = NOW() WHERE id = $2`
	_, err := s.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}
	return nil
}

// UpdateAgentHeartbeat records a heartbeat: sets status=connected,
// last_heartbeat=NOW(), and stores the reported agent version (empty
// version leaves the existing value intact).
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

// GetAgentByID looks up an agent by ID without tenant scoping (for WSS auth).
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

// CreateAgent creates a new agent record and returns the agent + raw API key (shown once).
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

// RotateAgentKey generates a new key and stores its hash in next_key_hash.
// Both the old key_hash and new next_key_hash are accepted until PromoteAgentKey is called.
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

// PromoteAgentKey moves next_key_hash to key_hash and clears next_key_hash.
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

// --- Targets (non-tenant-scoped) ---

// GetTargetByID looks up a target by ID without tenant scoping (for directive enrichment).
func (s *PostgresStore) GetTargetByID(ctx context.Context, id string) (*model.Target, error) {
	var t model.Target
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, type, identifier, config, environment, created_at, updated_at
		 FROM targets WHERE id = $1`, id).
		Scan(&t.ID, &t.TenantID, &t.AgentID, &t.Type, &t.Identifier, &t.Config, &t.Environment, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting target by id: %w", err)
	}
	return &t, nil
}

// --- Bundles ---

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

// --- Credentials ---

func (s *PostgresStore) GetCredentialsByTarget(ctx context.Context, targetID string) (json.RawMessage, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT encrypted_data FROM credentials WHERE target_id = $1 LIMIT 1`, targetID).
		Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting credentials: %w", err)
	}
	return json.RawMessage(data), nil
}

func (s *PostgresStore) CreateCredential(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO credentials (tenant_id, target_id, type, encrypted_data)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, targetID, credType, encryptedData).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating credential: %w", err)
	}
	return id, nil
}

// --- Scans (internal) ---

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

// --- Tenants (internal, not tenant-scoped) ---

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

func (s *PostgresStore) UpdateTenantStatus(ctx context.Context, id string, status string) error {
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

func (s *PostgresStore) UpdateTenantName(ctx context.Context, id string, name string) error {
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

// --- Agents (internal, cross-tenant) ---

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

// --- Stats ---

func (s *PostgresStore) GetDCStats(ctx context.Context) (*model.DCStats, error) {
	var stats model.DCStats
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE status = $1`, model.TenantStatusActive).Scan(&stats.TenantCount)
	if err != nil {
		return nil, fmt.Errorf("counting tenants: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&stats.AgentCount)
	if err != nil {
		return nil, fmt.Errorf("counting agents: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`).Scan(&stats.ScanCount)
	if err != nil {
		return nil, fmt.Errorf("counting scans: %w", err)
	}
	return &stats, nil
}

// ListAgents returns this tenant's agents (scoped via context).
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

// DeleteAgent removes an agent (tenant-scoped).
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

// UpsertCredential replaces the credential for a target (one credential
// per target via the credentials_target_unique index).
func (s *PostgresStore) UpsertCredential(ctx context.Context, tenantID, targetID, credType string, encryptedData []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO credentials (tenant_id, target_id, type, encrypted_data)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (target_id) DO UPDATE
		   SET type = EXCLUDED.type, encrypted_data = EXCLUDED.encrypted_data`,
		tenantID, targetID, credType, encryptedData)
	if err != nil {
		return fmt.Errorf("upserting credential: %w", err)
	}
	return nil
}

// DeleteCredential removes a credential for a target, scoped to tenant.
func (s *PostgresStore) DeleteCredential(ctx context.Context, tenantID, targetID string) error {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM credentials WHERE tenant_id = $1 AND target_id = $2`,
		tenantID, targetID)
	if err != nil {
		return fmt.Errorf("deleting credential: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// HasCredential reports whether a target has a credential, and its type.
// Used by the tenant UI to render 'credential set' vs 'no credential' without
// ever exposing the ciphertext.
func (s *PostgresStore) HasCredential(ctx context.Context, targetID string) (bool, string, error) {
	var credType string
	err := s.db.QueryRowContext(ctx,
		`SELECT type FROM credentials WHERE target_id = $1 LIMIT 1`, targetID).
		Scan(&credType)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("checking credential: %w", err)
	}
	return true, credType, nil
}

// --- Install tokens ---

// CreateInstallToken stores the hash of a one-time bootstrap token for this tenant.
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

// ConsumeInstallToken atomically marks the token used and returns the tenant
// it belongs to. Fails with sql.ErrNoRows if the token doesn't exist, is
// already used, or is expired.
func (s *PostgresStore) ConsumeInstallToken(ctx context.Context, tokenHash []byte, agentID string) (string, error) {
	var tenantID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE install_tokens
		   SET used_at = NOW(), used_agent_id = $2
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > NOW()
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
