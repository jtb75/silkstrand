package handler

import (
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// AssetHandler serves the tenant Assets page. Post-ADR-006 D2 the page
// reads `assets` (host-level) + `asset_endpoints` (port-level); P1 ships
// a working List over `assets` with the minimum filter surface so the
// frontend can at least render an empty page. Detail + coverage rollups
// land in P4.
type AssetHandler struct {
	store store.Store
}

func NewAssetHandler(s store.Store) *AssetHandler {
	return &AssetHandler{store: s}
}

// GET /api/v1/assets — minimal list. Source filter only in P1.
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	f := store.AssetFilter{
		Source: r.URL.Query().Get("source"),
	}
	items, total, err := h.store.ListAssets(r.Context(), f)
	if err != nil {
		slog.Error("listing assets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list assets")
		return
	}
	if items == nil {
		items = []model.Asset{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"page":      1,
		"page_size": len(items),
		"total":     total,
	})
}

// GET /api/v1/assets/{id} — P4 wires the join to asset_endpoints.
func (h *AssetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.store.GetAssetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if a == nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"asset": a, "events": []any{}})
}

// POST /api/v1/assets/{id}/promote — removed post-ADR-006. Promotion
// (turn a discovered endpoint into a compliance scan target) lands in P3
// as part of scan_definitions with scope_kind='asset_endpoint'. Returns
// 501 for now so the old UI path fails loudly rather than silently.
func (h *AssetHandler) Promote(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented,
		"asset promote is superseded by scan_definitions (scope=asset_endpoint); lands in P3")
}
