package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// AssetHandler serves the tenant Assets page (ADR 003 R1a).
// Read-only in R1a; the rule engine + promote-to-compliance flow lives
// in R1b.
type AssetHandler struct {
	store store.Store
}

func NewAssetHandler(s store.Store) *AssetHandler {
	return &AssetHandler{store: s}
}

// GET /api/v1/assets — list with filter chips + pagination.
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	f, err := parseAssetFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := h.store.ListAssets(r.Context(), f)
	if err != nil {
		slog.Error("listing assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}
	if items == nil {
		items = []model.DiscoveredAsset{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"page":      f.Page,
		"page_size": f.PageSize,
		"total":     total,
	})
}

// GET /api/v1/assets/{id} — single asset + recent events.
func (h *AssetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	asset, err := h.store.GetAssetByID(r.Context(), id)
	if err != nil {
		slog.Error("getting asset", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("events"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	events, err := h.store.ListAssetEventsByAsset(r.Context(), id, limit)
	if err != nil {
		slog.Error("listing asset events", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"asset":  asset,
		"events": events,
	})
}

func parseAssetFilter(r *http.Request) (store.AssetFilter, error) {
	q := r.URL.Query()
	f := store.AssetFilter{
		Service:          q.Get("service"),
		IPCIDR:           q.Get("ip"),
		Source:           q.Get("source"),
		ComplianceStatus: q.Get("compliance_status"),
		Q:                q.Get("q"),
		SortBy:           q.Get("sort_by"),
	}
	if v := q.Get("service_in"); v != "" {
		f.ServiceIn = strings.Split(v, ",")
	}
	if v := q.Get("cve_count_gte"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, &filterErr{"cve_count_gte must be a non-negative integer"}
		}
		f.HasCVECountGTE = n
	}
	if v := q.Get("new_since"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return f, err
		}
		f.NewSinceDuration = d
	}
	if v := q.Get("changed_since"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return f, err
		}
		f.ChangedSinceDuration = d
	}
	if v := q.Get("sort_dir"); v == "desc" {
		f.SortDesc = true
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.PageSize = n
		}
	}
	return f, nil
}

// parseDuration accepts either a Go duration ("168h") or a "Nd" shorthand.
func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n <= 0 {
			return 0, &filterErr{"duration must be positive"}
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, &filterErr{"invalid duration: " + err.Error()}
	}
	return d, nil
}

type filterErr struct{ msg string }

func (e *filterErr) Error() string { return e.msg }

// POST /api/v1/assets/{id}/promote
// Body: { bundle_id }
// Creates a compliance target from a candidate asset (R1b promote-from-
// suggestion). The asset's compliance_status flips to 'targeted'.
//
// Credentials still need to be set separately via the existing
// PUT /api/v1/targets/{tid}/credential flow — discovery doesn't bind
// creds (that's ADR 004 C1+).
func (h *AssetHandler) Promote(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	asset, err := h.store.GetAssetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	// ADR 003 D11 follow-up: refuse to promote an out-of-policy asset.
	// 'unknown' still allowed — missing snapshot must not block the
	// product; the agent's allowlist is the ultimate gate anyway.
	if asset.AllowlistStatus == model.AllowlistStatusOutOfPolicy {
		writeError(w, http.StatusConflict, "asset is out of the agent's scan allowlist; update /etc/silkstrand/scan-allowlist.yaml on the agent before promoting")
		return
	}
	var req struct {
		BundleID string `json:"bundle_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BundleID == "" {
		writeError(w, http.StatusBadRequest, "bundle_id is required")
		return
	}

	// Bundle determines the target type (engine probers map 1:1 to
	// service names). For unknown bundles we punt to 'host' which the
	// admin can adjust later — better to create the target than fail.
	targetType := model.TargetTypeHost
	if asset.Service != nil {
		switch *asset.Service {
		case "postgresql", "postgres":
			targetType = model.TargetTypePostgreSQL
		case "mssql", "ms-sql", "sqlserver":
			targetType = model.TargetTypeMSSQL
		case "mongodb", "mongo":
			targetType = model.TargetTypeMongoDB
		case "mysql":
			targetType = model.TargetTypeMySQL
		}
	}
	cfg := map[string]any{
		"host": asset.IP,
		"port": asset.Port,
	}
	cfgJSON, _ := json.Marshal(cfg)
	envPtr := ""
	if asset.Environment != nil {
		envPtr = *asset.Environment
	}
	target, err := h.store.CreateTarget(r.Context(), model.CreateTargetRequest{
		Type:        targetType,
		Identifier:  asset.IP,
		Config:      cfgJSON,
		Environment: envPtr,
	})
	if err != nil {
		slog.Error("creating target from asset", "asset", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}
	if err := h.store.SetTargetAsset(r.Context(), target.ID, asset.ID); err != nil {
		slog.Warn("wiring promoted target to asset", "target", target.ID, "asset", asset.ID, "error", err)
	}
	if err := h.store.SetAssetComplianceStatus(r.Context(), asset.ID, "targeted"); err != nil {
		slog.Warn("setting asset to targeted after promote", "asset", asset.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"target":   target,
		"bundle_id": req.BundleID,
	})
}
