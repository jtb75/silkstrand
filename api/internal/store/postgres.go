package store

import (
	"context"
	"database/sql"
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
		existing.Environment = *req.Environment
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
	defer tx.Rollback()

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
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, scan_id, control_id, title, status, severity, evidence, remediation, created_at
		 FROM scan_results WHERE scan_id = $1 ORDER BY control_id`, scanID)
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
