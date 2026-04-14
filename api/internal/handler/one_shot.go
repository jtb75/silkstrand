package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// OneShotScanHandler fans out a compliance bundle across a set of
// discovered assets (ADR 003 R1c-c / D13).
//
// v1 limitations:
//   - Agent must be specified by the caller. No automatic per-asset
//     agent routing.
//   - Each fanout creates a lightweight target row. We don't clean
//     them up in v1 (tracked as R1.5+ polish).
//   - No per-child completion updates on the parent (clients compute
//     progress from GET /scans?parent_one_shot_id=X). Roll-up to
//     one_shot_scans.completed_targets / status is a follow-up.
type OneShotScanHandler struct {
	store store.Store
	ps    *pubsub.PubSub
}

func NewOneShotScanHandler(s store.Store, ps *pubsub.PubSub) *OneShotScanHandler {
	return &OneShotScanHandler{store: s, ps: ps}
}

// POST /api/v1/one-shot-scans
// Body: {
//    bundle_id, agent_id,
//    asset_set_id?     | inline_predicate?,
//    max_concurrency?  (default 10), rate_limit_pps?, triggered_by?
// }
func (h *OneShotScanHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		BundleID        string          `json:"bundle_id"`
		AgentID         string          `json:"agent_id"`
		AssetSetID      *string         `json:"asset_set_id,omitempty"`
		InlinePredicate json.RawMessage `json:"inline_predicate,omitempty"`
		MaxConcurrency  int             `json:"max_concurrency,omitempty"`
		RateLimitPPS    *int            `json:"rate_limit_pps,omitempty"`
		TriggeredBy     string          `json:"triggered_by,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.BundleID == "" || req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "bundle_id and agent_id are required")
		return
	}
	if req.AssetSetID == nil && len(req.InlinePredicate) == 0 {
		writeError(w, http.StatusBadRequest, "asset_set_id or inline_predicate is required")
		return
	}

	// Resolve the predicate.
	predicate := req.InlinePredicate
	if req.AssetSetID != nil {
		set, err := h.store.GetAssetSet(r.Context(), *req.AssetSetID)
		if err != nil || set == nil {
			writeError(w, http.StatusNotFound, "asset set not found")
			return
		}
		predicate = set.Predicate
	}
	matches, err := resolveAssetSet(r, h.store, claims.TenantID, predicate)
	if err != nil {
		slog.Error("resolving asset set for one-shot", "error", err)
		writeError(w, http.StatusBadRequest, "predicate: "+err.Error())
		return
	}

	// Create the parent record first so children can reference it.
	maxC := req.MaxConcurrency
	if maxC <= 0 {
		maxC = 10
	}
	trig := req.TriggeredBy
	if trig == "" {
		trig = "user:" + claims.Email
	}
	parent, err := h.store.CreateOneShotScan(r.Context(), model.OneShotScan{
		TenantID:        claims.TenantID,
		BundleID:        req.BundleID,
		AssetSetID:      req.AssetSetID,
		InlinePredicate: req.InlinePredicate,
		MaxConcurrency:  maxC,
		RateLimitPPS:    req.RateLimitPPS,
		Status:          "running",
		TriggeredBy:     &trig,
	})
	if err != nil {
		slog.Error("creating one-shot parent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create one-shot")
		return
	}

	// Fan out. Synchronous for v1 — the dispatch loop is short (create
	// target, create scan, publish directive). The agent-side scanSem
	// serializes actual execution, so pushing 1000 directives at once
	// doesn't thrash the agent.
	dispatched := dispatchOneShotFanOut(r.Context(), h.store, h.ps,
		claims.TenantID, req.AgentID, req.BundleID, parent.ID, matches)
	if err := h.store.UpdateOneShotScanProgress(r.Context(), parent.ID, dispatched, "running"); err != nil {
		slog.Warn("updating one-shot progress", "id", parent.ID, "error", err)
	}
	parent.TotalTargets = &dispatched

	writeJSON(w, http.StatusCreated, parent)
}

// GET /api/v1/one-shot-scans
func (h *OneShotScanHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListOneShotScans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if items == nil {
		items = []model.OneShotScan{}
	}
	writeJSON(w, http.StatusOK, items)
}

// GET /api/v1/one-shot-scans/{id}
func (h *OneShotScanHandler) Get(w http.ResponseWriter, r *http.Request) {
	o, err := h.store.GetOneShotScan(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if o == nil {
		writeError(w, http.StatusNotFound, "one-shot not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// dispatchOneShotFanOut creates one target + one scan per matching
// asset and publishes the directive via Redis. Returns the count of
// dispatched scans.
func dispatchOneShotFanOut(
	ctx context.Context, s store.Store, ps *pubsub.PubSub,
	tenantID, agentID, bundleID, parentID string,
	assets []model.DiscoveredAsset,
) int {
	dispatched := 0
	for i := range assets {
		asset := &assets[i]
		targetType := serviceToTargetType(derefStringFn(asset.Service))
		cfg, _ := json.Marshal(map[string]any{
			"host": asset.IP,
			"port": asset.Port,
		})
		target, err := s.CreateTarget(ctx, model.CreateTargetRequest{
			AgentID:     &agentID,
			Type:        targetType,
			Identifier:  asset.IP,
			Config:      cfg,
			Environment: "__one_shot__",
		})
		if err != nil {
			slog.Warn("one-shot: creating target", "asset", asset.ID, "error", err)
			continue
		}
		if err := s.SetTargetAsset(ctx, target.ID, asset.ID); err != nil {
			slog.Warn("one-shot: wiring target to asset", "target", target.ID, "error", err)
		}
		scan, err := s.CreateScanForOneShot(ctx, tenantID, agentID, target.ID, bundleID, parentID)
		if err != nil {
			slog.Warn("one-shot: creating scan", "target", target.ID, "error", err)
			continue
		}
		if err := ps.PublishDirective(ctx, agentID, pubsub.Directive{
			ScanID:   scan.ID,
			ScanType: model.ScanTypeCompliance,
			BundleID: bundleID,
			TargetID: target.ID,
		}); err != nil {
			slog.Warn("one-shot: publishing directive", "scan", scan.ID, "error", err)
		}
		dispatched++
	}
	return dispatched
}

func derefStringFn(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func serviceToTargetType(service string) string {
	switch service {
	case "postgresql", "postgres":
		return model.TargetTypePostgreSQL
	case "mssql", "sqlserver":
		return model.TargetTypeMSSQL
	case "mongodb":
		return model.TargetTypeMongoDB
	case "mysql":
		return model.TargetTypeMySQL
	}
	return model.TargetTypeHost
}

// Used by the rule engine when ActionRunOneShotScan fires against a
// single asset that matched the rule. Materializes a one-shot of size
// 1 so the existing pipeline reuses.
func TriggerOneShotForAsset(ctx context.Context, s store.Store, ps *pubsub.PubSub,
	tenantID, agentID, bundleID string, asset *model.DiscoveredAsset, ruleName string) error {
	if asset == nil {
		return errors.New("asset nil")
	}
	trig := "rule:" + ruleName
	parent, err := s.CreateOneShotScan(ctx, model.OneShotScan{
		TenantID:       tenantID,
		BundleID:       bundleID,
		MaxConcurrency: 1,
		Status:         "running",
		TriggeredBy:    &trig,
	})
	if err != nil {
		return err
	}
	n := dispatchOneShotFanOut(ctx, s, ps, tenantID, agentID, bundleID, parent.ID,
		[]model.DiscoveredAsset{*asset})
	_ = s.UpdateOneShotScanProgress(ctx, parent.ID, n, "running")
	return nil
}
