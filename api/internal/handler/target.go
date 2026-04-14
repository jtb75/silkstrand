package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type TargetHandler struct {
	store store.Store
}

func NewTargetHandler(s store.Store) *TargetHandler {
	return &TargetHandler{store: s}
}

func (h *TargetHandler) List(w http.ResponseWriter, r *http.Request) {
	targets, err := h.store.ListTargets(r.Context())
	if err != nil {
		slog.Error("listing targets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	if targets == nil {
		targets = []model.Target{}
	}
	writeJSON(w, http.StatusOK, targets)
}

func (h *TargetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := h.store.GetTarget(r.Context(), id)
	if err != nil {
		slog.Error("getting target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (h *TargetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Type == "" || req.Identifier == "" {
		writeError(w, http.StatusBadRequest, "type and identifier are required")
		return
	}
	if !model.IsValidTargetType(req.Type) {
		writeError(w, http.StatusBadRequest,
			"unsupported target type: "+req.Type)
		return
	}

	target, err := h.store.CreateTarget(r.Context(), req)
	if err != nil {
		slog.Error("creating target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}

	// D6 unification: register a discovered_assets row for this target so
	// future discovery passes enrich it in place rather than duplicating.
	// Best-effort: log and continue if asset wiring fails — the target row
	// itself is good. R1a's first discovery pass reconciles missing wires.
	claims := middleware.GetClaims(r.Context())
	if claims != nil && claims.TenantID != "" {
		ip := assetIPFromTarget(req.Type, req.Identifier)
		port := assetPortFromConfig(req.Config)
		var env *string
		if req.Environment != "" {
			e := req.Environment
			env = &e
		}
		if asset, err := h.store.UpsertManualAsset(r.Context(), claims.TenantID, ip, port, env); err != nil {
			slog.Warn("upserting manual asset for target", "target_id", target.ID, "error", err)
		} else if err := h.store.SetTargetAsset(r.Context(), target.ID, asset.ID); err != nil {
			slog.Warn("wiring target to asset", "target_id", target.ID, "asset_id", asset.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, target)
}

// assetIPFromTarget extracts a usable IP for the discovered_assets row.
// Engine targets put host in config; network_range targets put it in
// identifier. Sentinel 0.0.0.0 when we can't tell — first discovery
// pass reconciles.
func assetIPFromTarget(targetType, identifier string) string {
	if targetType == model.TargetTypeNetworkRange || targetType == model.TargetTypeCIDR || targetType == model.TargetTypeHost {
		return identifier
	}
	return "0.0.0.0"
}

// assetPortFromConfig pulls "port" out of the engine-shaped config JSON.
// Falls back to 0 (host-level row) if absent or unparseable.
func assetPortFromConfig(cfg json.RawMessage) int {
	if len(cfg) == 0 {
		return 0
	}
	var m struct {
		Port any `json:"port"`
	}
	if err := json.Unmarshal(cfg, &m); err != nil {
		return 0
	}
	switch v := m.Port.(type) {
	case float64:
		return int(v)
	case string:
		var n int
		_, _ = fmtSscan(v, &n)
		return n
	}
	return 0
}

// fmtSscan is a tiny shim so we don't import fmt here just for this.
func fmtSscan(s string, p *int) (int, error) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		*p = (*p)*10 + int(c-'0')
	}
	return 1, nil
}

func (h *TargetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	target, err := h.store.UpdateTarget(r.Context(), id, req)
	if err != nil {
		slog.Error("updating target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (h *TargetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteTarget(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
