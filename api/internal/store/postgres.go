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
		`SELECT id, tenant_id, agent_id, target_id, bundle_id, scan_type, status, started_at, completed_at, created_at
		 FROM scans WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing scans: %w", err)
	}
	defer rows.Close()

	var scans []model.Scan
	for rows.Next() {
		var sc model.Scan
		if err := rows.Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.ScanType, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt); err != nil {
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
		`SELECT id, tenant_id, agent_id, target_id, bundle_id, scan_type, status, started_at, completed_at, created_at
		 FROM scans WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.ScanType, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
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

	scanType := req.ScanType
	if scanType == "" {
		scanType = model.ScanTypeCompliance
	}

	var sc model.Scan
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO scans (tenant_id, agent_id, target_id, bundle_id, scan_type, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, agent_id, target_id, bundle_id, scan_type, status, started_at, completed_at, created_at`,
		tenantID, target.AgentID, req.TargetID, req.BundleID, scanType, model.ScanStatusPending).
		Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.ScanType, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
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

// DeleteScan removes a scan (tenant-scoped). scan_results rows are removed
// via ON DELETE CASCADE. Callers are expected to refuse deletion of
// scans in a running state.
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

func (s *PostgresStore) GetScanByID(ctx context.Context, id string) (*model.Scan, error) {
	var sc model.Scan
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, agent_id, target_id, bundle_id, scan_type, status, started_at, completed_at, created_at
		 FROM scans WHERE id = $1`, id).
		Scan(&sc.ID, &sc.TenantID, &sc.AgentID, &sc.TargetID, &sc.BundleID, &sc.ScanType, &sc.Status, &sc.StartedAt, &sc.CompletedAt, &sc.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting scan by id: %w", err)
	}
	return &sc, nil
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

// --- Credential Sources (ADR 004 C0) ---
//
// These methods back the future refactor of the /api/v1/targets/{id}/credential
// surface onto a pluggable credential_sources table. No handler calls them
// yet; they land additively so a follow-up PR can swap the read/write path
// without schema changes.

// CreateCredentialSource inserts a credential_sources row and returns its id.
func (s *PostgresStore) CreateCredentialSource(ctx context.Context, tenantID, srcType string, config json.RawMessage) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO credential_sources (tenant_id, type, config)
		 VALUES ($1, $2, $3) RETURNING id`,
		tenantID, srcType, config).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating credential source: %w", err)
	}
	return id, nil
}

// GetCredentialSource returns a credential_sources row by id, or (nil, nil)
// if not found.
func (s *PostgresStore) GetCredentialSource(ctx context.Context, id string) (*model.CredentialSource, error) {
	var cs model.CredentialSource
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, type, config, created_at, updated_at
		 FROM credential_sources WHERE id = $1`, id).
		Scan(&cs.ID, &cs.TenantID, &cs.Type, &cs.Config, &cs.CreatedAt, &cs.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting credential source: %w", err)
	}
	return &cs, nil
}

// GetCredentialSourceByTarget resolves the credential source linked to a
// target via targets.credential_source_id. Returns (nil, nil) if the target
// has no linked source.
func (s *PostgresStore) GetCredentialSourceByTarget(ctx context.Context, targetID string) (*model.CredentialSource, error) {
	var cs model.CredentialSource
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.id, cs.tenant_id, cs.type, cs.config, cs.created_at, cs.updated_at
		 FROM credential_sources cs
		 JOIN targets t ON t.credential_source_id = cs.id
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

// UpdateCredentialSourceConfig replaces the config JSON of an existing
// source and bumps updated_at.
func (s *PostgresStore) UpdateCredentialSourceConfig(ctx context.Context, id string, config json.RawMessage) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE credential_sources SET config = $1, updated_at = NOW() WHERE id = $2`,
		config, id)
	if err != nil {
		return fmt.Errorf("updating credential source: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteCredentialSource removes a credential_sources row. Any targets
// still referencing it have their FK nulled by ON DELETE SET NULL.
func (s *PostgresStore) DeleteCredentialSource(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM credential_sources WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting credential source: %w", err)
	}
	return nil
}

// SetTargetCredentialSource points a target at a credential source.
func (s *PostgresStore) SetTargetCredentialSource(ctx context.Context, targetID, sourceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET credential_source_id = $1, updated_at = NOW() WHERE id = $2`,
		sourceID, targetID)
	if err != nil {
		return fmt.Errorf("setting target credential source: %w", err)
	}
	return nil
}

// ClearTargetCredentialSource unlinks a target from any credential source.
func (s *PostgresStore) ClearTargetCredentialSource(ctx context.Context, targetID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET credential_source_id = NULL, updated_at = NOW() WHERE id = $1`,
		targetID)
	if err != nil {
		return fmt.Errorf("clearing target credential source: %w", err)
	}
	return nil
}

// UpsertStaticCredentialSource ensures a `static`-type credential_sources row
// exists for the target and is linked via targets.credential_source_id. If a
// static source already exists for the target, its config is replaced in
// place. Runs in a transaction so the FK and source row stay consistent.
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

	var existingID sql.NullString
	var existingType sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT cs.id, cs.type
		 FROM targets t
		 LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		 WHERE t.id = $1`, targetID).
		Scan(&existingID, &existingType)
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
		return fmt.Errorf("target %s has non-static credential source; C0 cannot overwrite", targetID)
	default:
		var newID string
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO credential_sources (tenant_id, type, config)
			 VALUES ($1, $2, $3) RETURNING id`,
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

// GetStaticCredentialForTarget returns (encryptedBytes, credType) for a
// target. Prefers credential_sources via targets.credential_source_id when
// present and of type `static`; falls back to the legacy credentials table.
func (s *PostgresStore) GetStaticCredentialForTarget(ctx context.Context, targetID string) ([]byte, string, error) {
	var (
		srcType sql.NullString
		cfgRaw  sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.type, cs.config::text
		 FROM targets t
		 LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		 WHERE t.id = $1`, targetID).
		Scan(&srcType, &cfgRaw)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", fmt.Errorf("looking up credential source: %w", err)
	}

	if srcType.Valid && srcType.String == model.CredentialSourceTypeStatic && cfgRaw.Valid {
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

	// Fallback to legacy table (handles rollback-window edge cases).
	var (
		legacyType string
		legacyData []byte
	)
	err = s.db.QueryRowContext(ctx,
		`SELECT type, encrypted_data FROM credentials WHERE target_id = $1 LIMIT 1`,
		targetID).Scan(&legacyType, &legacyData)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("legacy credential read: %w", err)
	}
	return legacyData, legacyType, nil
}

// HasCredentialForTarget is the presence-check equivalent of
// GetStaticCredentialForTarget (credential_sources preferred, legacy fallback).
func (s *PostgresStore) HasCredentialForTarget(ctx context.Context, targetID string) (bool, string, error) {
	var (
		srcType sql.NullString
		cfgRaw  sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT cs.type, cs.config::text
		 FROM targets t
		 LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		 WHERE t.id = $1`, targetID).
		Scan(&srcType, &cfgRaw)
	if err != nil && err != sql.ErrNoRows {
		return false, "", fmt.Errorf("checking credential source: %w", err)
	}
	if srcType.Valid && srcType.String == model.CredentialSourceTypeStatic && cfgRaw.Valid {
		var cfg model.StaticCredentialConfig
		if err := json.Unmarshal([]byte(cfgRaw.String), &cfg); err != nil {
			return false, "", fmt.Errorf("decoding static credential config: %w", err)
		}
		return true, cfg.Type, nil
	}

	var legacyType string
	err = s.db.QueryRowContext(ctx,
		`SELECT type FROM credentials WHERE target_id = $1 LIMIT 1`, targetID).
		Scan(&legacyType)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("legacy credential presence check: %w", err)
	}
	return true, legacyType, nil
}

// DeleteCredentialForTarget removes both the credential_sources row (when
// linked via static) and the legacy credentials row. Returns sql.ErrNoRows
// only when neither surface had a credential to remove.
func (s *PostgresStore) DeleteCredentialForTarget(ctx context.Context, tenantID, targetID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceID sql.NullString
	var sourceType sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT cs.id, cs.type
		 FROM targets t
		 LEFT JOIN credential_sources cs ON cs.id = t.credential_source_id
		 WHERE t.id = $1`, targetID).Scan(&sourceID, &sourceType); err != nil {
		return fmt.Errorf("looking up credential source: %w", err)
	}

	removedSource := false
	if sourceID.Valid && sourceType.String == model.CredentialSourceTypeStatic {
		if _, err := tx.ExecContext(ctx,
			`UPDATE targets SET credential_source_id = NULL, updated_at = NOW() WHERE id = $1`,
			targetID); err != nil {
			return fmt.Errorf("clearing target credential source: %w", err)
		}
		res, err := tx.ExecContext(ctx,
			`DELETE FROM credential_sources WHERE id = $1`, sourceID.String)
		if err != nil {
			return fmt.Errorf("deleting credential source: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removedSource = true
		}
	}

	legacyRes, err := tx.ExecContext(ctx,
		`DELETE FROM credentials WHERE tenant_id = $1 AND target_id = $2`,
		tenantID, targetID)
	if err != nil {
		return fmt.Errorf("deleting legacy credential: %w", err)
	}
	removedLegacy := false
	if n, _ := legacyRes.RowsAffected(); n > 0 {
		removedLegacy = true
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	if !removedSource && !removedLegacy {
		return sql.ErrNoRows
	}
	return nil
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

// ListBundlesForTenant returns bundles available to a tenant: either global
// (tenant_id IS NULL) or explicitly owned by this tenant.
func (s *PostgresStore) ListBundlesForTenant(ctx context.Context, tenantID string) ([]model.Bundle, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at
		   FROM bundles
		  WHERE tenant_id IS NULL OR tenant_id = $1
		  ORDER BY name, version`, tenantID)
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

// UpsertBundle creates or updates a bundle by id. Used to seed global bundles
// via the backoffice internal API.
func (s *PostgresStore) UpsertBundle(ctx context.Context, b model.Bundle) (*model.Bundle, error) {
	var out model.Bundle
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO bundles (id, tenant_id, name, version, framework, target_type, gcs_path, signature)
		 VALUES (COALESCE(NULLIF($1, '')::uuid, uuid_generate_v4()), $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO UPDATE
		   SET name = EXCLUDED.name,
		       version = EXCLUDED.version,
		       framework = EXCLUDED.framework,
		       target_type = EXCLUDED.target_type,
		       gcs_path = EXCLUDED.gcs_path,
		       signature = EXCLUDED.signature
		 RETURNING id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at`,
		b.ID, b.TenantID, b.Name, b.Version, b.Framework, b.TargetType, b.GCSPath, b.Signature).
		Scan(&out.ID, &out.TenantID, &out.Name, &out.Version, &out.Framework, &out.TargetType,
			&out.GCSPath, &out.Signature, &out.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting bundle: %w", err)
	}
	return &out, nil
}

// --- Recon (ADR 003 R1a): discovered_assets + asset_events ---

const assetSelectCols = `id, tenant_id, host(ip), port, hostname, service, version,
		technologies, cves, compliance_status, source, environment,
		first_seen, last_seen, last_scan_id, missed_scan_count, metadata,
		created_at, updated_at`

func scanAsset(scanner interface {
	Scan(dest ...interface{}) error
}, a *model.DiscoveredAsset) error {
	return scanner.Scan(
		&a.ID, &a.TenantID, &a.IP, &a.Port, &a.Hostname, &a.Service, &a.Version,
		&a.Technologies, &a.CVEs, &a.ComplianceStatus, &a.Source, &a.Environment,
		&a.FirstSeen, &a.LastSeen, &a.LastScanID, &a.MissedScanCount, &a.Metadata,
		&a.CreatedAt, &a.UpdatedAt,
	)
}

// UpsertDiscoveredAsset inserts or updates a (tenant, ip, port) row from
// a discovery scan. Returns the new row plus the previous row (nil if
// this is a fresh asset) so the caller can derive asset_events by diff.
// Runs in a transaction.
func (s *PostgresStore) UpsertDiscoveredAsset(ctx context.Context, scanID string, in DiscoveredAssetInput) (*model.DiscoveredAsset, *model.DiscoveredAsset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Read previous state (may be empty).
	var oldAsset model.DiscoveredAsset
	hadOld := true
	err = tx.QueryRowContext(ctx,
		`SELECT `+assetSelectCols+` FROM discovered_assets
		 WHERE tenant_id = $1 AND ip = $2 AND port = $3 FOR UPDATE`,
		in.TenantID, in.IP, in.Port).Scan(
		&oldAsset.ID, &oldAsset.TenantID, &oldAsset.IP, &oldAsset.Port, &oldAsset.Hostname,
		&oldAsset.Service, &oldAsset.Version, &oldAsset.Technologies, &oldAsset.CVEs,
		&oldAsset.ComplianceStatus, &oldAsset.Source, &oldAsset.Environment,
		&oldAsset.FirstSeen, &oldAsset.LastSeen, &oldAsset.LastScanID, &oldAsset.MissedScanCount,
		&oldAsset.Metadata, &oldAsset.CreatedAt, &oldAsset.UpdatedAt)
	if err == sql.ErrNoRows {
		hadOld = false
	} else if err != nil {
		return nil, nil, fmt.Errorf("reading old asset: %w", err)
	}

	tech := in.Technologies
	if len(tech) == 0 {
		tech = json.RawMessage("[]")
	}
	cves := in.CVEs
	if len(cves) == 0 {
		cves = json.RawMessage("[]")
	}

	var hostname, service, version *string
	if in.Hostname != "" {
		hostname = &in.Hostname
	}
	if in.Service != "" {
		service = &in.Service
	}
	if in.Version != "" {
		version = &in.Version
	}

	var newAsset model.DiscoveredAsset
	err = tx.QueryRowContext(ctx,
		`INSERT INTO discovered_assets
		   (tenant_id, ip, port, hostname, service, version, technologies, cves,
		    source, environment, first_seen, last_seen, last_scan_id, missed_scan_count)
		 VALUES ($1, $2::INET, $3, $4, $5, $6, $7, $8, 'discovered', $9, NOW(), NOW(), $10, 0)
		 ON CONFLICT (tenant_id, ip, port) DO UPDATE SET
		   hostname          = COALESCE(EXCLUDED.hostname, discovered_assets.hostname),
		   service           = COALESCE(EXCLUDED.service,  discovered_assets.service),
		   version           = COALESCE(EXCLUDED.version,  discovered_assets.version),
		   technologies      = EXCLUDED.technologies,
		   cves              = EXCLUDED.cves,
		   environment       = COALESCE(EXCLUDED.environment, discovered_assets.environment),
		   last_seen         = NOW(),
		   last_scan_id      = EXCLUDED.last_scan_id,
		   missed_scan_count = 0,
		   updated_at        = NOW()
		 RETURNING `+assetSelectCols,
		in.TenantID, in.IP, in.Port, hostname, service, version, tech, cves,
		in.Environment, scanID).Scan(
		&newAsset.ID, &newAsset.TenantID, &newAsset.IP, &newAsset.Port, &newAsset.Hostname,
		&newAsset.Service, &newAsset.Version, &newAsset.Technologies, &newAsset.CVEs,
		&newAsset.ComplianceStatus, &newAsset.Source, &newAsset.Environment,
		&newAsset.FirstSeen, &newAsset.LastSeen, &newAsset.LastScanID, &newAsset.MissedScanCount,
		&newAsset.Metadata, &newAsset.CreatedAt, &newAsset.UpdatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("upserting asset: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}
	if !hadOld {
		return &newAsset, nil, nil
	}
	return &newAsset, &oldAsset, nil
}

// AppendAssetEvents inserts a batch of events. Caller derives them from
// the (old, new) tuple returned by UpsertDiscoveredAsset.
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
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())`)
	if err != nil {
		return fmt.Errorf("prepare events insert: %w", err)
	}
	defer stmt.Close()
	for _, e := range events {
		payload := e.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		if _, err := stmt.ExecContext(ctx, e.TenantID, e.AssetID, e.ScanID, e.EventType, e.Severity, payload); err != nil {
			return fmt.Errorf("inserting event: %w", err)
		}
	}
	return tx.Commit()
}

// GetAssetByID returns one asset, tenant-scoped.
func (s *PostgresStore) GetAssetByID(ctx context.Context, id string) (*model.DiscoveredAsset, error) {
	tenantID := TenantID(ctx)
	var a model.DiscoveredAsset
	err := scanAsset(s.db.QueryRowContext(ctx,
		`SELECT `+assetSelectCols+` FROM discovered_assets WHERE id = $1 AND tenant_id = $2`,
		id, tenantID), &a)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting asset: %w", err)
	}
	return &a, nil
}

// ListAssets paginates the asset table for the calling tenant.
// Filter is translated to WHERE clauses; CVE count uses a partial index.
func (s *PostgresStore) ListAssets(ctx context.Context, f AssetFilter) ([]model.DiscoveredAsset, int, error) {
	tenantID := TenantID(ctx)
	if f.PageSize <= 0 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
	if f.Page <= 0 {
		f.Page = 1
	}

	where := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	add := func(clause string, val interface{}) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Service != "" {
		add("service = $%d", f.Service)
	}
	if len(f.ServiceIn) > 0 {
		add("service = ANY($%d)", f.ServiceIn)
	}
	if f.IPCIDR != "" {
		add("ip <<= $%d::cidr", f.IPCIDR)
	}
	if f.Source != "" {
		add("source = $%d", f.Source)
	}
	if f.ComplianceStatus != "" {
		add("compliance_status = $%d", f.ComplianceStatus)
	}
	if f.HasCVECountGTE > 0 {
		add("jsonb_array_length(cves) >= $%d", f.HasCVECountGTE)
	}
	if f.NewSinceDuration > 0 {
		add("first_seen >= NOW() - make_interval(hours => $%d)", int(f.NewSinceDuration.Hours()))
	}
	if f.ChangedSinceDuration > 0 {
		add("last_seen >= NOW() - make_interval(hours => $%d)", int(f.ChangedSinceDuration.Hours()))
	}
	if f.Q != "" {
		q := "%" + f.Q + "%"
		add("(hostname ILIKE $%d OR service ILIKE $%[1]d OR version ILIKE $%[1]d OR host(ip) ILIKE $%[1]d)", q)
	}

	sortCol := "last_seen"
	switch f.SortBy {
	case "first_seen", "ip", "service":
		sortCol = f.SortBy
	case "cve_count":
		sortCol = "jsonb_array_length(cves)"
	}
	dir := "ASC"
	if f.SortDesc || f.SortBy == "" || f.SortBy == "last_seen" || f.SortBy == "first_seen" {
		dir = "DESC"
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM discovered_assets WHERE "+joinWith(where, " AND "), args...).
		Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting assets: %w", err)
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	query := fmt.Sprintf(
		"SELECT %s FROM discovered_assets WHERE %s ORDER BY %s %s LIMIT $%d OFFSET $%d",
		assetSelectCols, joinWith(where, " AND "), sortCol, dir,
		len(args)-1, len(args))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing assets: %w", err)
	}
	defer rows.Close()
	var out []model.DiscoveredAsset
	for rows.Next() {
		var a model.DiscoveredAsset
		if err := scanAsset(rows, &a); err != nil {
			return nil, 0, fmt.Errorf("scan asset: %w", err)
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}

func joinWith(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// ListAssetEventsByAsset returns the most recent N events for an asset.
func (s *PostgresStore) ListAssetEventsByAsset(ctx context.Context, assetID string, limit int) ([]model.AssetEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, asset_id, scan_id, event_type, severity, payload, occurred_at
		 FROM asset_events WHERE asset_id = $1 ORDER BY occurred_at DESC LIMIT $2`,
		assetID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing asset events: %w", err)
	}
	defer rows.Close()
	var out []model.AssetEvent
	for rows.Next() {
		var e model.AssetEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.AssetID, &e.ScanID, &e.EventType, &e.Severity, &e.Payload, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpsertManualAsset is what target-creation calls (D6 unification): make
// sure a discovered_assets row exists for (tenant_id, ip, port) with
// source='manual' and return it.
func (s *PostgresStore) UpsertManualAsset(ctx context.Context, tenantID, ip string, port int, environment *string) (*model.DiscoveredAsset, error) {
	var a model.DiscoveredAsset
	err := scanAsset(s.db.QueryRowContext(ctx,
		`INSERT INTO discovered_assets (tenant_id, ip, port, source, environment)
		 VALUES ($1, $2::INET, $3, 'manual', $4)
		 ON CONFLICT (tenant_id, ip, port) DO UPDATE SET
		   environment = COALESCE(EXCLUDED.environment, discovered_assets.environment),
		   updated_at  = NOW()
		 RETURNING `+assetSelectCols,
		tenantID, ip, port, environment), &a)
	if err != nil {
		return nil, fmt.Errorf("upserting manual asset: %w", err)
	}
	return &a, nil
}

// SetTargetAsset wires a target row to a discovered_assets row.
func (s *PostgresStore) SetTargetAsset(ctx context.Context, targetID, assetID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET asset_id = $1, updated_at = NOW() WHERE id = $2`,
		assetID, targetID)
	if err != nil {
		return fmt.Errorf("setting target asset: %w", err)
	}
	return nil
}
