// Package main is the DC API server entrypoint. Post-ADR-006/007 P1 the
// rule engine + notify dispatcher + allowlist tracker are temporarily
// offline — their orchestration lands in P2 alongside the new asset /
// asset_endpoint ingest path. Until then the WSS handler logs and drops
// asset_discovered + allowlist_snapshot messages with a TODO.
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

	pgStore, err := store.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pgStore.Close()

	if err := runMigrations(pgStore.DB()); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	ps, err := pubsub.New(cfg.RedisURL)
	if err != nil {
		slog.Warn("redis not available, pub/sub disabled", "error", err)
		ps = nil
	} else {
		defer ps.Close()
	}

	websocket.AllowedOrigins = cfg.AllowedOrigins
	hub := websocket.NewHub()
	hub.OnMessage = buildOnMessage(pgStore, ps)

	// Handlers — surviving surface
	healthH := handler.NewHealthHandler(pgStore, redisPingFunc(ps))
	targetH := handler.NewTargetHandler(pgStore)
	scanH := handler.NewScanHandler(pgStore, ps, hub)
	agentH := handler.NewAgentHandler(hub, pgStore, ps, cfg.CredentialEncryptionKey)
	agentsH := handler.NewAgentsHandler(pgStore, hub, ps, cfg.AgentReleasesURL)
	credsH := handler.NewCredentialsHandler(pgStore, cfg.CredentialEncryptionKey)
	probeH := handler.NewProbeHandler(pgStore, ps, cfg.CredentialEncryptionKey)
	bundlesH := handler.NewBundlesHandler(pgStore)
	internalH := handler.NewInternalHandler(pgStore, cfg.CredentialEncryptionKey)
	assetH := handler.NewAssetHandler(pgStore)

	// Handlers — new asset-first surface (collections has working impls;
	// the rest return 501 in P1).
	collectionsH := handler.NewCollectionsHandler(pgStore)
	findingsH := handler.NewFindingsHandler(nil)
	scanDefsH := handler.NewScanDefinitionsHandler(nil)
	credMapH := handler.NewCredentialMappingsHandler(nil)
	dashH := handler.NewDashboardHandler(nil)
	rulesH := handler.NewCorrelationRulesHandler(nil)

	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("GET /healthz", healthH.Healthz)
	mux.HandleFunc("GET /readyz", healthH.Readyz)

	// Agent WSS
	mux.HandleFunc("GET /ws/agent", agentH.Connect)

	// Agent bootstrap + self-delete (authed by agent key, not tenant JWT)
	mux.HandleFunc("POST /api/v1/agents/bootstrap", agentsH.Bootstrap)
	mux.HandleFunc("DELETE /api/v1/agents/self", agentH.SelfDelete)

	// Internal (backoffice)
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("PUT /internal/v1/bundles", internalH.UpsertBundle)
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

	// Tenant API
	apiMux := http.NewServeMux()

	// Targets (narrowed to CIDR / network_range per ADR 006 D8)
	apiMux.HandleFunc("GET /api/v1/targets", targetH.List)
	apiMux.HandleFunc("POST /api/v1/targets", targetH.Create)
	apiMux.HandleFunc("GET /api/v1/targets/{id}", targetH.Get)
	apiMux.HandleFunc("PUT /api/v1/targets/{id}", targetH.Update)
	apiMux.HandleFunc("DELETE /api/v1/targets/{id}", targetH.Delete)

	apiMux.HandleFunc("GET /api/v1/targets/{id}/credential", credsH.Get)
	apiMux.HandleFunc("PUT /api/v1/targets/{id}/credential", credsH.Put)
	apiMux.HandleFunc("DELETE /api/v1/targets/{id}/credential", credsH.Delete)
	apiMux.HandleFunc("POST /api/v1/targets/{id}/probe", probeH.Probe)

	// Bundles
	apiMux.HandleFunc("GET /api/v1/bundles", bundlesH.List)

	// Agents
	apiMux.HandleFunc("GET /api/v1/agents/downloads", agentsH.Downloads)
	apiMux.HandleFunc("GET /api/v1/agents", agentsH.List)
	apiMux.HandleFunc("POST /api/v1/agents", agentsH.Create)
	apiMux.HandleFunc("GET /api/v1/agents/{id}", agentsH.Get)
	apiMux.HandleFunc("GET /api/v1/agents/{id}/allowlist", agentsH.GetAllowlist)
	apiMux.HandleFunc("POST /api/v1/agents/{id}/rotate-key", agentsH.RotateKey)
	apiMux.HandleFunc("POST /api/v1/agents/{id}/upgrade", agentsH.Upgrade)
	apiMux.HandleFunc("DELETE /api/v1/agents/{id}", agentsH.Delete)
	apiMux.HandleFunc("POST /api/v1/agents/install-tokens", agentsH.CreateInstallToken)

	// Scans (ad-hoc debug path; scan_definitions is the durable surface)
	apiMux.HandleFunc("POST /api/v1/scans", scanH.Create)
	apiMux.HandleFunc("GET /api/v1/scans", scanH.List)
	apiMux.HandleFunc("GET /api/v1/scans/{id}", scanH.Get)
	apiMux.HandleFunc("DELETE /api/v1/scans/{id}", scanH.Delete)

	// Assets (ADR 006 D2) — list works against empty `assets`, detail
	// returns 404 post-migration-017 because all rows are gone; promote
	// returns 501 (superseded by scan_definitions).
	apiMux.HandleFunc("GET /api/v1/assets", assetH.List)
	apiMux.HandleFunc("GET /api/v1/assets/{id}", assetH.Get)
	apiMux.HandleFunc("POST /api/v1/assets/{id}/promote", assetH.Promote)

	// Collections (ADR 006 D5) — working CRUD in P1.
	apiMux.HandleFunc("GET /api/v1/collections", collectionsH.List)
	apiMux.HandleFunc("POST /api/v1/collections", collectionsH.Create)
	apiMux.HandleFunc("POST /api/v1/collections/preview", collectionsH.Preview)
	apiMux.HandleFunc("GET /api/v1/collections/{id}", collectionsH.Get)
	apiMux.HandleFunc("PUT /api/v1/collections/{id}", collectionsH.Update)
	apiMux.HandleFunc("DELETE /api/v1/collections/{id}", collectionsH.Delete)

	// Correlation rules — 501 stubs (P2).
	apiMux.HandleFunc("GET /api/v1/correlation-rules", rulesH.List)
	apiMux.HandleFunc("POST /api/v1/correlation-rules", rulesH.Create)
	apiMux.HandleFunc("GET /api/v1/correlation-rules/{id}", rulesH.Get)
	apiMux.HandleFunc("PUT /api/v1/correlation-rules/{id}", rulesH.Update)
	apiMux.HandleFunc("DELETE /api/v1/correlation-rules/{id}", rulesH.Delete)

	// Findings (ADR 007 D1) — 501 stubs (P3).
	apiMux.HandleFunc("GET /api/v1/findings", findingsH.List)
	apiMux.HandleFunc("GET /api/v1/findings/{id}", findingsH.Get)
	apiMux.HandleFunc("POST /api/v1/findings/{id}/suppress", findingsH.Suppress)
	apiMux.HandleFunc("POST /api/v1/findings/{id}/reopen", findingsH.Reopen)

	// Scan definitions (ADR 007 D3) — 501 stubs (P3).
	apiMux.HandleFunc("GET /api/v1/scan-definitions", scanDefsH.List)
	apiMux.HandleFunc("POST /api/v1/scan-definitions", scanDefsH.Create)
	apiMux.HandleFunc("GET /api/v1/scan-definitions/{id}", scanDefsH.Get)
	apiMux.HandleFunc("PUT /api/v1/scan-definitions/{id}", scanDefsH.Update)
	apiMux.HandleFunc("DELETE /api/v1/scan-definitions/{id}", scanDefsH.Delete)
	apiMux.HandleFunc("POST /api/v1/scan-definitions/{id}/execute", scanDefsH.Execute)
	apiMux.HandleFunc("POST /api/v1/scan-definitions/{id}/enable", scanDefsH.Enable)
	apiMux.HandleFunc("POST /api/v1/scan-definitions/{id}/disable", scanDefsH.Disable)

	// Credential mappings — 501 (P5).
	apiMux.HandleFunc("GET /api/v1/credential-mappings", credMapH.List)
	apiMux.HandleFunc("POST /api/v1/credential-mappings", credMapH.Create)
	apiMux.HandleFunc("GET /api/v1/credential-mappings/{id}", credMapH.Get)
	apiMux.HandleFunc("DELETE /api/v1/credential-mappings/{id}", credMapH.Delete)

	// Dashboard — 501 (P5).
	apiMux.HandleFunc("GET /api/v1/dashboard", dashH.Get)

	authedAPI := middleware.Auth(cfg.JWTSecret)(middleware.Tenant(pgStore)(apiMux))
	mux.Handle("/api/", authedAPI)

	corsOrigins := strings.Join(cfg.AllowedOrigins, ",")
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      middleware.CORS(corsOrigins)(middleware.Logging(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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

// buildOnMessage routes agent → server WSS messages. P1 preserves the
// routing table but neuters the asset_discovered + allowlist_snapshot
// paths (the tables they wrote to are gone); P2 reintroduces ingest
// against assets + asset_endpoints.
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
			// P1: findings/scan_results storage is gone. Mark the scan
			// completed so the UI state machine advances; P3 wires
			// findings ingest write-through per ADR 007 D2.
			var wrapper struct {
				ScanID string `json:"scan_id"`
			}
			if err := json.Unmarshal(msg.Payload, &wrapper); err == nil && wrapper.ScanID != "" {
				if err := s.UpdateScanStatus(ctx, wrapper.ScanID, model.ScanStatusCompleted); err != nil {
					slog.Error("updating scan to completed", "scan_id", wrapper.ScanID, "error", err)
				}
				slog.Info("scan_results received (P1 drop; findings ingest lands in P3)",
					"agent_id", agentID, "scan_id", wrapper.ScanID)
			}

		case websocket.TypeScanError:
			var payload websocket.ScanErrorPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				slog.Error("parsing scan_error payload", "agent_id", agentID, "error", err)
				return
			}
			if err := s.FailScan(ctx, payload.ScanID, payload.Error); err != nil {
				slog.Error("updating scan to failed", "scan_id", payload.ScanID, "error", err)
			}
			slog.Warn("scan failed", "agent_id", agentID, "scan_id", payload.ScanID, "error", payload.Error)

		case websocket.TypeProbeResult:
			var result websocket.ProbeResultPayload
			if err := json.Unmarshal(msg.Payload, &result); err != nil {
				slog.Error("parsing probe_result payload", "agent_id", agentID, "error", err)
				return
			}
			if ps != nil {
				if err := ps.PublishProbeResult(ctx, result.ProbeID, msg.Payload); err != nil {
					slog.Error("publishing probe_result", "probe_id", result.ProbeID, "error", err)
				}
			}

		case websocket.TypeHeartbeat:
			var hb websocket.HeartbeatPayload
			if err := json.Unmarshal(msg.Payload, &hb); err != nil {
				slog.Debug("parsing heartbeat payload", "agent_id", agentID, "error", err)
			}
			if err := s.UpdateAgentHeartbeat(ctx, agentID, hb.Version); err != nil {
				slog.Error("updating agent heartbeat", "agent_id", agentID, "error", err)
			}

		case websocket.TypeAllowlistSnapshot:
			// TODO(P2): reintroduce agent_allowlists via the new asset /
			// asset_endpoints shape. For now we log + drop.
			slog.Info("allowlist_snapshot received (P1 drop; asset-first rewiring lands in P2)",
				"agent_id", agentID)

		case websocket.TypeAssetDiscovered:
			// TODO(P2): upsert into assets + asset_endpoints + emit
			// asset_events + run EvaluateAsset through the new
			// collection-aware rule engine. P1 logs + drops.
			slog.Info("asset_discovered received (P1 drop; P2 rewires ingest against assets + asset_endpoints)",
				"agent_id", agentID)

		case websocket.TypeDiscoveryCompleted:
			var payload websocket.DiscoveryCompletedPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				slog.Error("parsing discovery_completed payload", "agent_id", agentID, "error", err)
				return
			}
			if err := s.UpdateScanStatus(ctx, payload.ScanID, model.ScanStatusCompleted); err != nil {
				slog.Error("updating discovery scan to completed", "scan_id", payload.ScanID, "error", err)
			}
			slog.Info("discovery completed", "agent_id", agentID, "scan_id", payload.ScanID,
				"assets_found", payload.AssetsFound, "hosts_scanned", payload.HostsScanned)

		default:
			slog.Warn("unknown message type from agent", "agent_id", agentID, "type", msg.Type)
		}
	}
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
