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
	"github.com/jtb75/silkstrand/api/internal/notify"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/rules"
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
	hub.OnMessage = buildOnMessage(pgStore, ps, hub)

	// Handlers
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
	rulesH := handler.NewCorrelationRulesHandler(pgStore)
	notifDispatcher := notify.New(pgStore, cfg.CredentialEncryptionKey)
	globalNotifier = notifDispatcher // exposed for runRuleActions (see bottom of file)
	globalPubSub = ps                // exposed for rule-driven one-shot dispatch
	channelsH := handler.NewNotificationChannelsHandler(pgStore, cfg.CredentialEncryptionKey)
	assetSetsH := handler.NewAssetSetsHandler(pgStore)
	oneShotH := handler.NewOneShotScanHandler(pgStore, ps)

	// Router
	mux := http.NewServeMux()

	// Public routes (no auth)
	mux.HandleFunc("GET /healthz", healthH.Healthz)
	mux.HandleFunc("GET /readyz", healthH.Readyz)

	// Agent WebSocket (agent auth — key-based, separate from user auth)
	mux.HandleFunc("GET /ws/agent", agentH.Connect)

	// Agent bootstrap (public; authed by install token in the body)
	mux.HandleFunc("POST /api/v1/agents/bootstrap", agentsH.Bootstrap)

	// Agent self-delete (authed by the agent's own API key, not a
	// tenant user JWT — so it can be called from the uninstall flow).
	mux.HandleFunc("DELETE /api/v1/agents/self", agentH.SelfDelete)

	// Internal API routes (backoffice access via API key)
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
	apiMux.HandleFunc("POST /api/v1/targets/{id}/probe", probeH.Probe)

	apiMux.HandleFunc("GET /api/v1/bundles", bundlesH.List)

	apiMux.HandleFunc("GET /api/v1/agents/downloads", agentsH.Downloads)
	apiMux.HandleFunc("GET /api/v1/agents", agentsH.List)
	apiMux.HandleFunc("POST /api/v1/agents", agentsH.Create)
	apiMux.HandleFunc("GET /api/v1/agents/{id}", agentsH.Get)
	apiMux.HandleFunc("POST /api/v1/agents/{id}/rotate-key", agentsH.RotateKey)
	apiMux.HandleFunc("POST /api/v1/agents/{id}/upgrade", agentsH.Upgrade)
	apiMux.HandleFunc("DELETE /api/v1/agents/{id}", agentsH.Delete)
	apiMux.HandleFunc("POST /api/v1/agents/install-tokens", agentsH.CreateInstallToken)
	apiMux.HandleFunc("POST /api/v1/scans", scanH.Create)
	apiMux.HandleFunc("GET /api/v1/scans", scanH.List)
	apiMux.HandleFunc("GET /api/v1/scans/{id}", scanH.Get)
	apiMux.HandleFunc("DELETE /api/v1/scans/{id}", scanH.Delete)

	apiMux.HandleFunc("GET /api/v1/assets", assetH.List)
	apiMux.HandleFunc("GET /api/v1/assets/{id}", assetH.Get)
	apiMux.HandleFunc("POST /api/v1/assets/{id}/promote", assetH.Promote)

	apiMux.HandleFunc("GET /api/v1/correlation-rules", rulesH.List)
	apiMux.HandleFunc("POST /api/v1/correlation-rules", rulesH.Create)
	apiMux.HandleFunc("GET /api/v1/correlation-rules/{id}", rulesH.Get)
	apiMux.HandleFunc("PUT /api/v1/correlation-rules/{id}", rulesH.Update)
	apiMux.HandleFunc("DELETE /api/v1/correlation-rules/{id}", rulesH.Delete)

	apiMux.HandleFunc("GET /api/v1/notification-channels", channelsH.List)
	apiMux.HandleFunc("POST /api/v1/notification-channels", channelsH.Create)
	apiMux.HandleFunc("GET /api/v1/notification-channels/{id}", channelsH.Get)
	apiMux.HandleFunc("PUT /api/v1/notification-channels/{id}", channelsH.Update)
	apiMux.HandleFunc("DELETE /api/v1/notification-channels/{id}", channelsH.Delete)

	apiMux.HandleFunc("GET /api/v1/asset-sets", assetSetsH.List)
	apiMux.HandleFunc("POST /api/v1/asset-sets", assetSetsH.Create)
	apiMux.HandleFunc("POST /api/v1/asset-sets/preview", assetSetsH.PreviewAdhoc)
	apiMux.HandleFunc("GET /api/v1/asset-sets/{id}", assetSetsH.Get)
	apiMux.HandleFunc("PUT /api/v1/asset-sets/{id}", assetSetsH.Update)
	apiMux.HandleFunc("DELETE /api/v1/asset-sets/{id}", assetSetsH.Delete)
	apiMux.HandleFunc("GET /api/v1/asset-sets/{id}/preview", assetSetsH.Preview)

	apiMux.HandleFunc("POST /api/v1/one-shot-scans", oneShotH.Create)
	apiMux.HandleFunc("GET /api/v1/one-shot-scans", oneShotH.List)
	apiMux.HandleFunc("GET /api/v1/one-shot-scans/{id}", oneShotH.Get)

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
func buildOnMessage(s store.Store, ps *pubsub.PubSub, hub *websocket.Hub) func(agentID string, msg websocket.Message) {
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

		case websocket.TypeProbeResult:
			var result websocket.ProbeResultPayload
			if err := json.Unmarshal(msg.Payload, &result); err != nil {
				slog.Error("parsing probe_result payload", "agent_id", agentID, "error", err)
				return
			}
			// Publish to Redis so whichever instance owns the originating
			// HTTP probe handler can pick the result up.
			if err := ps.PublishProbeResult(ctx, result.ProbeID, msg.Payload); err != nil {
				slog.Error("publishing probe_result to redis", "probe_id", result.ProbeID, "error", err)
			}

		case websocket.TypeHeartbeat:
			var hb websocket.HeartbeatPayload
			if err := json.Unmarshal(msg.Payload, &hb); err != nil {
				// Tolerate missing payload — older agents may not send one.
				slog.Debug("parsing heartbeat payload", "agent_id", agentID, "error", err)
			}
			if err := s.UpdateAgentHeartbeat(ctx, agentID, hb.Version); err != nil {
				slog.Error("updating agent heartbeat", "agent_id", agentID, "error", err)
			}

		case websocket.TypeAssetDiscovered:
			handleAssetDiscovered(ctx, s, agentID, msg.Payload)

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

// handleAssetDiscovered processes a batch of asset findings from an agent
// during a discovery scan. Each asset is upserted into discovered_assets
// and any deltas vs. the prior row are appended to asset_events. Per
// ADR 003 D9 we process inline (no buffering) so the Assets page sees
// progress live.
func handleAssetDiscovered(ctx context.Context, s store.Store, agentID string, payload json.RawMessage) {
	var batch websocket.AssetDiscoveredPayload
	if err := json.Unmarshal(payload, &batch); err != nil {
		slog.Error("parsing asset_discovered payload", "agent_id", agentID, "error", err)
		return
	}

	scan, err := s.GetScanByID(ctx, batch.ScanID)
	if err != nil || scan == nil {
		slog.Error("loading discovery scan", "scan_id", batch.ScanID, "error", err)
		return
	}

	// First message flips pending → running.
	if scan.Status == model.ScanStatusPending {
		if err := s.UpdateScanStatus(ctx, scan.ID, model.ScanStatusRunning); err != nil {
			slog.Error("updating discovery scan to running", "scan_id", scan.ID, "error", err)
		}
	}

	// Load the tenant's active asset_discovered rules once per batch.
	// R1b — engine evaluates after each asset upsert.
	activeRules, err := s.ListActiveRules(ctx, scan.TenantID, model.RuleTriggerAssetDiscovered)
	if err != nil {
		slog.Warn("loading active rules for asset_discovered", "tenant", scan.TenantID, "error", err)
		// Continue without rules — ingest must not fail because rules can't load.
	}

	for _, a := range batch.Assets {
		in := store.DiscoveredAssetInput{
			TenantID:     scan.TenantID,
			IP:           a.IP,
			Port:         a.Port,
			Hostname:     a.Hostname,
			Service:      a.Service,
			Version:      a.Version,
			Technologies: a.Technologies,
			CVEs:         a.CVEs,
		}
		newAsset, oldAsset, err := s.UpsertDiscoveredAsset(ctx, scan.ID, in)
		if err != nil {
			slog.Error("upserting discovered asset", "scan_id", scan.ID, "ip", a.IP, "port", a.Port, "error", err)
			continue
		}
		events := deriveAssetEvents(scan.TenantID, scan.ID, oldAsset, newAsset)
		if err := s.AppendAssetEvents(ctx, events); err != nil {
			slog.Error("appending asset events", "asset_id", newAsset.ID, "error", err)
		}
		runRuleActions(ctx, s, activeRules, newAsset)
	}
}

// runRuleActions evaluates the loaded rules against the asset and
// executes each fired action. R1b implements suggest_target and
// auto_create_target; notify and run_one_shot_scan are R1c.
func runRuleActions(ctx context.Context, s store.Store, ruleSet []model.CorrelationRule, asset *model.DiscoveredAsset) {
	if asset == nil || len(ruleSet) == 0 {
		return
	}
	fired := rules.EvaluateAsset(ruleSet, asset)
	for _, act := range fired {
		switch act.Type {
		case rules.ActionSuggestTarget:
			bundleID := act.BundleID()
			if bundleID == "" {
				slog.Warn("suggest_target action missing bundle", "rule", act.RuleName, "asset", asset.ID)
				continue
			}
			if err := s.SetAssetSuggestion(ctx, asset.ID, act.RuleName, bundleID); err != nil {
				slog.Warn("recording asset suggestion", "asset", asset.ID, "error", err)
				continue
			}
			if err := s.SetAssetComplianceStatus(ctx, asset.ID, "candidate"); err != nil {
				slog.Warn("setting asset to candidate", "asset", asset.ID, "error", err)
			}
			slog.Info("rule.fired",
				"rule", act.RuleName, "action", act.Type,
				"asset", asset.ID, "bundle", bundleID)

		case rules.ActionNotify:
			if globalNotifier == nil {
				continue
			}
			channel, _ := act.Params["channel"].(string)
			if channel == "" {
				continue
			}
			severity, _ := act.Params["severity"].(string)
			if severity == "" {
				severity = notify.SeverityInfo
			}
			title, _ := act.Params["title"].(string)
			if title == "" {
				title = "Rule " + act.RuleName + " fired"
			}
			message, _ := act.Params["message"].(string)
			globalNotifier.DispatchAsync(notify.Event{
				TenantID:    asset.TenantID,
				ChannelName: channel,
				Severity:    severity,
				Title:       title,
				Message:     message,
				AssetID:     asset.ID,
				RuleID:      act.RuleID,
				RuleName:    act.RuleName,
				Payload: map[string]any{
					"ip":      asset.IP,
					"port":    asset.Port,
					"service": derefString(asset.Service),
					"version": derefString(asset.Version),
				},
			})
			slog.Info("rule.fired",
				"rule", act.RuleName, "action", act.Type,
				"asset", asset.ID, "channel", channel)

		case rules.ActionRunOneShotScan:
			if globalPubSub == nil {
				continue
			}
			bundleID := act.BundleID()
			agentID, _ := act.Params["agent_id"].(string)
			if bundleID == "" || agentID == "" {
				slog.Warn("run_one_shot_scan missing bundle_id or agent_id",
					"rule", act.RuleName, "asset", asset.ID)
				continue
			}
			if err := handler.TriggerOneShotForAsset(ctx, s, globalPubSub,
				asset.TenantID, agentID, bundleID, asset, act.RuleName); err != nil {
				slog.Warn("run_one_shot_scan dispatch", "rule", act.RuleName, "asset", asset.ID, "error", err)
			} else {
				slog.Info("rule.fired",
					"rule", act.RuleName, "action", act.Type,
					"asset", asset.ID, "bundle", bundleID)
			}

		case rules.ActionAutoCreateTarget:
			bundleID := act.BundleID()
			if bundleID == "" {
				slog.Warn("auto_create_target action missing bundle", "rule", act.RuleName, "asset", asset.ID)
				continue
			}
			if err := s.SetAssetComplianceStatus(ctx, asset.ID, "targeted"); err != nil {
				slog.Warn("setting asset to targeted", "asset", asset.ID, "error", err)
			}
			// Future: when the target-creation handler accepts an
			// asset_id directly, wire that here. For R1b we record
			// the intent on the asset; the suggest_target UI flow
			// (Approve button) is the human path that actually
			// materializes the target row. auto_create remains
			// degenerate until R1c lands the credential-resolver
			// integration that lets us create targets without the
			// admin filling in connection details.
			if err := s.SetAssetSuggestion(ctx, asset.ID, act.RuleName, bundleID); err != nil {
				slog.Warn("recording auto-target intent", "asset", asset.ID, "error", err)
			}
			slog.Info("rule.fired",
				"rule", act.RuleName, "action", act.Type,
				"asset", asset.ID, "bundle", bundleID)
		}
	}
}

// deriveAssetEvents diffs the old and new asset rows and emits the
// corresponding asset_events. Algorithm follows the API plan §7 Q3.
func deriveAssetEvents(tenantID, scanID string, old, new *model.DiscoveredAsset) []model.AssetEvent {
	if new == nil {
		return nil
	}
	var events []model.AssetEvent
	mk := func(eventType string, payload json.RawMessage) {
		sid := scanID
		events = append(events, model.AssetEvent{
			TenantID:  tenantID,
			AssetID:   new.ID,
			ScanID:    &sid,
			EventType: eventType,
			Payload:   payload,
		})
	}

	if old == nil {
		mk(model.AssetEventNewAsset, mustJSON(map[string]any{
			"service": derefString(new.Service),
			"version": derefString(new.Version),
			"port":    new.Port,
		}))
		return events
	}

	if derefString(old.Service) != derefString(new.Service) || derefString(old.Version) != derefString(new.Version) {
		mk(model.AssetEventVersionChanged, mustJSON(map[string]any{
			"from_service": derefString(old.Service), "to_service": derefString(new.Service),
			"from_version": derefString(old.Version), "to_version": derefString(new.Version),
		}))
	}

	added, removed := diffCVEIDs(old.CVEs, new.CVEs)
	for _, id := range added {
		mk(model.AssetEventNewCVE, mustJSON(map[string]string{"cve_id": id}))
	}
	for _, id := range removed {
		mk(model.AssetEventCVEResolved, mustJSON(map[string]string{"cve_id": id}))
	}
	return events
}

func diffCVEIDs(oldCVEs, newCVEs json.RawMessage) (added, removed []string) {
	oldIDs := cveIDSet(oldCVEs)
	newIDs := cveIDSet(newCVEs)
	for id := range newIDs {
		if !oldIDs[id] {
			added = append(added, id)
		}
	}
	for id := range oldIDs {
		if !newIDs[id] {
			removed = append(removed, id)
		}
	}
	return added, removed
}

func cveIDSet(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return out
	}
	for _, item := range arr {
		if id, ok := item["id"].(string); ok && id != "" {
			out[id] = true
		}
	}
	return out
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// globalNotifier is set during server init so runRuleActions can
// dispatch without threading the dispatcher through signature churn.
// There is exactly one Dispatcher per process; it's safe to stash
// in a package var.
var globalNotifier *notify.Dispatcher

// globalPubSub mirrors globalNotifier — exposes Redis dispatch to the
// rule action for one-shot scans (which creates per-asset scans and
// must publish directives).
var globalPubSub *pubsub.PubSub
