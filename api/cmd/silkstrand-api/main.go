package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/jtb75/silkstrand/api/internal/config"
	"github.com/jtb75/silkstrand/api/internal/handler"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Database
	pgStore, err := store.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pgStore.Close()

	// Run migrations
	if err := runMigrations(pgStore.DB()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Redis pub/sub
	ps, err := pubsub.New(cfg.RedisURL)
	if err != nil {
		slog.Warn("redis not available, pub/sub disabled", "error", err)
		ps = nil
	} else {
		defer ps.Close()
	}

	// WebSocket hub
	websocket.AllowedOrigins = cfg.AllowedOrigins
	hub := websocket.NewHub()

	// Wire up agent message handling
	hub.OnMessage = buildOnMessage(pgStore, ps)

	// Handlers
	healthH := handler.NewHealthHandler(pgStore, redisPingFunc(ps))
	targetH := handler.NewTargetHandler(pgStore)
	scanH := handler.NewScanHandler(pgStore, ps, hub)
	agentH := handler.NewAgentHandler(hub, pgStore, ps, cfg.CredentialEncryptionKey)
	agentsH := handler.NewAgentsHandler(pgStore, cfg.AgentReleasesURL)
	credsH := handler.NewCredentialsHandler(pgStore, cfg.CredentialEncryptionKey)
	internalH := handler.NewInternalHandler(pgStore, cfg.CredentialEncryptionKey)

	// Router
	mux := http.NewServeMux()

	// Public routes (no auth)
	mux.HandleFunc("GET /healthz", healthH.Healthz)
	mux.HandleFunc("GET /readyz", healthH.Readyz)

	// Agent WebSocket (agent auth — key-based, separate from user auth)
	mux.HandleFunc("GET /ws/agent", agentH.Connect)

	// Internal API routes (backoffice access via API key)
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("POST /internal/v1/tenants", internalH.CreateTenant)
	internalMux.HandleFunc("GET /internal/v1/tenants", internalH.ListTenants)
	internalMux.HandleFunc("GET /internal/v1/tenants/{id}", internalH.GetTenant)
	internalMux.HandleFunc("PUT /internal/v1/tenants/{id}", internalH.UpdateTenant)
	internalMux.HandleFunc("DELETE /internal/v1/tenants/{id}", internalH.DeleteTenant)
	internalMux.HandleFunc("GET /internal/v1/agents", internalH.ListAgents)
	internalMux.HandleFunc("GET /internal/v1/stats", internalH.GetStats)
	internalMux.HandleFunc("POST /internal/v1/credentials", internalH.CreateCredential)
	authedInternal := middleware.InternalAuth(cfg.InternalAPIKey)(internalMux)
	mux.Handle("/internal/", authedInternal)

	// Authenticated API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/targets", targetH.List)
	apiMux.HandleFunc("POST /api/v1/targets", targetH.Create)
	apiMux.HandleFunc("GET /api/v1/targets/{id}", targetH.Get)
	apiMux.HandleFunc("PUT /api/v1/targets/{id}", targetH.Update)
	apiMux.HandleFunc("DELETE /api/v1/targets/{id}", targetH.Delete)

	apiMux.HandleFunc("GET /api/v1/targets/{id}/credential", credsH.Get)
	apiMux.HandleFunc("PUT /api/v1/targets/{id}/credential", credsH.Put)
	apiMux.HandleFunc("DELETE /api/v1/targets/{id}/credential", credsH.Delete)

	apiMux.HandleFunc("GET /api/v1/agents/downloads", agentsH.Downloads)
	apiMux.HandleFunc("GET /api/v1/agents", agentsH.List)
	apiMux.HandleFunc("POST /api/v1/agents", agentsH.Create)
	apiMux.HandleFunc("GET /api/v1/agents/{id}", agentsH.Get)
	apiMux.HandleFunc("POST /api/v1/agents/{id}/rotate-key", agentsH.RotateKey)
	apiMux.HandleFunc("DELETE /api/v1/agents/{id}", agentsH.Delete)
	apiMux.HandleFunc("POST /api/v1/scans", scanH.Create)
	apiMux.HandleFunc("GET /api/v1/scans", scanH.List)
	apiMux.HandleFunc("GET /api/v1/scans/{id}", scanH.Get)

	// Apply auth + tenant middleware to API routes
	authedAPI := middleware.Auth(cfg.JWTSecret)(middleware.Tenant(pgStore)(apiMux))
	mux.Handle("/api/", authedAPI)

	// Apply CORS + logging to all routes. CORS must be outermost so that
	// OPTIONS preflight replies carry the Access-Control-* headers and
	// aren't swallowed by auth middleware.
	corsOrigins := strings.Join(cfg.AllowedOrigins, ",")
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.CORS(corsOrigins)(middleware.Logging(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		errCh <- server.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return server.Shutdown(ctx)
}

// buildOnMessage returns the hub.OnMessage callback that handles agent messages.
func buildOnMessage(s store.Store, ps *pubsub.PubSub) func(agentID string, msg websocket.Message) {
	return func(agentID string, msg websocket.Message) {
		ctx := context.Background()

		switch msg.Type {
		case websocket.TypeScanStarted:
			var payload websocket.ScanStartedPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				slog.Error("parsing scan_started payload", "agent_id", agentID, "error", err)
				return
			}
			if err := s.UpdateScanStatus(ctx, payload.ScanID, model.ScanStatusRunning); err != nil {
				slog.Error("updating scan to running", "scan_id", payload.ScanID, "error", err)
			}
			slog.Info("scan started", "agent_id", agentID, "scan_id", payload.ScanID)

		case websocket.TypeScanResults:
			handleScanResults(ctx, s, ps, agentID, msg.Payload)

		case websocket.TypeScanError:
			var payload websocket.ScanErrorPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				slog.Error("parsing scan_error payload", "agent_id", agentID, "error", err)
				return
			}
			if err := s.UpdateScanStatus(ctx, payload.ScanID, model.ScanStatusFailed); err != nil {
				slog.Error("updating scan to failed", "scan_id", payload.ScanID, "error", err)
			}
			slog.Warn("scan failed", "agent_id", agentID, "scan_id", payload.ScanID, "error", payload.Error)

		case websocket.TypeHeartbeat:
			if err := s.UpdateAgentStatus(ctx, agentID, model.AgentStatusConnected); err != nil {
				slog.Error("updating agent heartbeat", "agent_id", agentID, "error", err)
			}

		default:
			slog.Warn("unknown message type from agent", "agent_id", agentID, "type", msg.Type)
		}
	}
}

// ScanResultsMessage wraps the standard results schema with a scan_id.
type ScanResultsMessage struct {
	ScanID  string          `json:"scan_id"`
	Results json.RawMessage `json:"results"`
}

// BundleResults is the standard results schema output from bundles.
type BundleResults struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Controls      []struct {
		ID          string          `json:"id"`
		Title       string          `json:"title"`
		Status      string          `json:"status"`
		Severity    string          `json:"severity,omitempty"`
		Evidence    json.RawMessage `json:"evidence,omitempty"`
		Remediation string          `json:"remediation,omitempty"`
	} `json:"controls"`
}

func handleScanResults(ctx context.Context, s store.Store, ps *pubsub.PubSub, agentID string, payload json.RawMessage) {
	var msg ScanResultsMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		slog.Error("parsing scan_results payload", "agent_id", agentID, "error", err)
		return
	}

	var results BundleResults
	if err := json.Unmarshal(msg.Results, &results); err != nil {
		slog.Error("parsing bundle results", "scan_id", msg.ScanID, "error", err)
		if err := s.UpdateScanStatus(ctx, msg.ScanID, model.ScanStatusFailed); err != nil {
			slog.Error("updating scan to failed", "scan_id", msg.ScanID, "error", err)
		}
		return
	}

	// Convert to model.ScanResult slice
	scanResults := make([]model.ScanResult, 0, len(results.Controls))
	for _, c := range results.Controls {
		scanResults = append(scanResults, model.ScanResult{
			ScanID:      msg.ScanID,
			ControlID:   c.ID,
			Title:       c.Title,
			Status:      c.Status,
			Severity:    c.Severity,
			Evidence:    c.Evidence,
			Remediation: c.Remediation,
		})
	}

	// Store results
	if err := s.CreateScanResults(ctx, msg.ScanID, scanResults); err != nil {
		slog.Error("storing scan results", "scan_id", msg.ScanID, "error", err)
		if err := s.UpdateScanStatus(ctx, msg.ScanID, model.ScanStatusFailed); err != nil {
			slog.Error("updating scan to failed", "scan_id", msg.ScanID, "error", err)
		}
		return
	}

	// Update scan status to completed
	if err := s.UpdateScanStatus(ctx, msg.ScanID, model.ScanStatusCompleted); err != nil {
		slog.Error("updating scan to completed", "scan_id", msg.ScanID, "error", err)
		return
	}

	// Publish progress for real-time UI
	if ps != nil {
		_ = ps.PublishScanProgress(ctx, msg.ScanID, model.ScanStatusCompleted)
	}

	slog.Info("scan completed", "agent_id", agentID, "scan_id", msg.ScanID, "controls", len(scanResults))
}

func runMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(store.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("creating migration source: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("creating migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	slog.Info("migrations complete")
	return nil
}

func redisPingFunc(ps *pubsub.PubSub) func(context.Context) error {
	if ps == nil {
		return nil
	}
	return ps.Ping
}
