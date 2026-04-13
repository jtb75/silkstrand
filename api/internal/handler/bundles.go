package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type BundlesHandler struct {
	store store.Store
}

func NewBundlesHandler(s store.Store) *BundlesHandler {
	return &BundlesHandler{store: s}
}

// GET /api/v1/bundles (tenant-authed)
// Returns bundles available to this tenant: global ones + any tenant-owned.
func (h *BundlesHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	bundles, err := h.store.ListBundlesForTenant(r.Context(), claims.TenantID)
	if err != nil {
		slog.Error("listing bundles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list bundles")
		return
	}
	if bundles == nil {
		bundles = []model.Bundle{}
	}
	writeJSON(w, http.StatusOK, bundles)
}

// UpsertBundle — internal (backoffice-authed). Used to seed global bundles
// (tenant_id NULL) that every tenant can use. Payload mirrors model.Bundle.
// Note: this lives on InternalHandler in main.go to reuse X-API-Key auth.
type UpsertBundleRequest struct {
	ID         string  `json:"id"`
	TenantID   *string `json:"tenant_id,omitempty"`
	Name       string  `json:"name"`
	Version    string  `json:"version"`
	Framework  string  `json:"framework"`
	TargetType string  `json:"target_type"`
	GCSPath    *string `json:"gcs_path,omitempty"`
	Signature  *string `json:"signature,omitempty"`
}

// InternalUpsertBundle is mounted on the /internal/v1 mux.
func (h *InternalHandler) UpsertBundle(w http.ResponseWriter, r *http.Request) {
	var req UpsertBundleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Version == "" || req.Framework == "" || req.TargetType == "" {
		writeError(w, http.StatusBadRequest, "name, version, framework, and target_type are required")
		return
	}
	b := model.Bundle{
		ID:         req.ID,
		TenantID:   req.TenantID,
		Name:       req.Name,
		Version:    req.Version,
		Framework:  req.Framework,
		TargetType: req.TargetType,
		GCSPath:    req.GCSPath,
		Signature:  req.Signature,
	}
	out, err := h.store.UpsertBundle(r.Context(), b)
	if err != nil {
		slog.Error("upserting bundle", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert bundle")
		return
	}
	writeJSON(w, http.StatusOK, out)
}
