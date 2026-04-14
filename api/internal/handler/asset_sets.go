package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/rules"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// AssetSetsHandler serves CRUD + preview for D13 asset sets.
type AssetSetsHandler struct {
	store store.Store
}

func NewAssetSetsHandler(s store.Store) *AssetSetsHandler {
	return &AssetSetsHandler{store: s}
}

func (h *AssetSetsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListAssetSets(r.Context())
	if err != nil {
		slog.Error("list asset sets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if items == nil {
		items = []model.AssetSet{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AssetSetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	set, err := h.store.GetAssetSet(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if set == nil {
		writeError(w, http.StatusNotFound, "asset set not found")
		return
	}
	writeJSON(w, http.StatusOK, set)
}

// POST /api/v1/asset-sets
// Body: { name, description?, predicate }
func (h *AssetSetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	set, err := h.parse(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.store.CreateAssetSet(r.Context(), *set)
	if err != nil {
		slog.Error("create asset set", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// PUT /api/v1/asset-sets/{id}
func (h *AssetSetsHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, err := h.store.GetAssetSet(r.Context(), r.PathValue("id"))
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "asset set not found")
		return
	}
	set, err := h.parse(r, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	set.ID = existing.ID
	out, err := h.store.UpdateAssetSet(r.Context(), *set)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AssetSetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAssetSet(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/asset-sets/{id}/preview — runs the saved predicate and
// returns the matching count + a sample (first 25). Useful for the UI
// to show "this set matches 12 assets" before saving.
func (h *AssetSetsHandler) Preview(w http.ResponseWriter, r *http.Request) {
	set, err := h.store.GetAssetSet(r.Context(), r.PathValue("id"))
	if err != nil || set == nil {
		writeError(w, http.StatusNotFound, "asset set not found")
		return
	}
	matches, err := resolveAssetSet(r, h.store, set.TenantID, set.Predicate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	limit := 25
	sample := matches
	if len(sample) > limit {
		sample = sample[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(matches),
		"sample": sample,
	})
}

// POST /api/v1/asset-sets/preview — same, but accepts an ad-hoc
// predicate in the body instead of a saved id. Lets the UI preview as
// the admin composes.
func (h *AssetSetsHandler) PreviewAdhoc(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Predicate json.RawMessage `json:"predicate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Predicate) == 0 {
		writeError(w, http.StatusBadRequest, "predicate is required")
		return
	}
	matches, err := resolveAssetSet(r, h.store, claims.TenantID, req.Predicate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed: "+err.Error())
		return
	}
	limit := 25
	sample := matches
	if len(sample) > limit {
		sample = sample[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(matches),
		"sample": sample,
	})
}

func (h *AssetSetsHandler) parse(r *http.Request, isNew bool) (*model.AssetSet, error) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		return nil, errors.New("unauthorized")
	}
	var req struct {
		Name        string          `json:"name"`
		Description *string         `json:"description"`
		Predicate   json.RawMessage `json:"predicate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.New("invalid request body")
	}
	if isNew && req.Name == "" {
		return nil, errors.New("name is required")
	}
	if len(req.Predicate) == 0 {
		return nil, errors.New("predicate is required")
	}
	// Sanity-check predicate by running it against a zero asset — the
	// call either returns a clean bool/nil or surfaces a parse error.
	if _, err := rules.Match(req.Predicate, &model.DiscoveredAsset{}); err != nil {
		return nil, errors.New("predicate: " + err.Error())
	}
	return &model.AssetSet{
		TenantID:    claims.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Predicate:   req.Predicate,
	}, nil
}

// resolveAssetSet loads all tenant assets and filters in memory using
// the predicate matcher. Fine at R1 scale; revisit with a SQL compiler
// if any tenant's asset table grows beyond ~10k rows.
func resolveAssetSet(r *http.Request, s store.Store, tenantID string, predicate json.RawMessage) ([]model.DiscoveredAsset, error) {
	all, err := s.ListAllAssetsForTenant(r.Context(), tenantID)
	if err != nil {
		return nil, err
	}
	var out []model.DiscoveredAsset
	for i := range all {
		ok, err := rules.Match(predicate, &all[i])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, all[i])
		}
	}
	return out, nil
}
