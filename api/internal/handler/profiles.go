package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// ProfilesHandler serves the compliance profiles CRUD surface
// (ADR 010 D9 — Level 3A).
type ProfilesHandler struct {
	store store.Store
}

func NewProfilesHandler(s store.Store) *ProfilesHandler {
	return &ProfilesHandler{store: s}
}

type createProfileRequest struct {
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	BaseFramework *string `json:"base_framework,omitempty"`
}

type updateProfileRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type setControlsRequest struct {
	ControlIDs []string `json:"control_ids"`
}

// List handles GET /api/v1/compliance-profiles.
func (h *ProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListComplianceProfiles(r.Context())
	if err != nil {
		slog.Error("listing compliance profiles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list compliance profiles")
		return
	}
	if items == nil {
		items = []model.ComplianceProfile{}
	}
	writeJSON(w, http.StatusOK, items)
}

// Get handles GET /api/v1/compliance-profiles/{id}.
func (h *ProfilesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil {
		slog.Error("getting compliance profile", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get profile")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Create handles POST /api/v1/compliance-profiles.
func (h *ProfilesHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())

	var req createProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	p := model.ComplianceProfile{
		Name:          req.Name,
		Description:   req.Description,
		BaseFramework: req.BaseFramework,
	}
	if claims != nil && claims.UserID != "" {
		uid := claims.UserID
		p.CreatedBy = &uid
	}

	out, err := h.store.CreateComplianceProfile(r.Context(), p)
	if err != nil {
		slog.Error("creating compliance profile", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create profile")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Update handles PUT /api/v1/compliance-profiles/{id}.
func (h *ProfilesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated := *existing
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Description != nil {
		updated.Description = req.Description
	}

	out, err := h.store.UpdateComplianceProfile(r.Context(), id, updated)
	if err != nil {
		slog.Error("updating compliance profile", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	if out == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Delete handles DELETE /api/v1/compliance-profiles/{id}.
func (h *ProfilesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteComplianceProfile(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetControls handles POST /api/v1/compliance-profiles/{id}/controls.
func (h *ProfilesHandler) SetControls(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	var req setControlsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.SetProfileControls(r.Context(), id, req.ControlIDs); err != nil {
		slog.Error("setting profile controls", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to set controls")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetControls handles GET /api/v1/compliance-profiles/{id}/controls.
func (h *ProfilesHandler) GetControls(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	controls, err := h.store.GetProfileControls(r.Context(), id)
	if err != nil {
		slog.Error("getting profile controls", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get controls")
		return
	}
	if controls == nil {
		controls = []string{}
	}
	writeJSON(w, http.StatusOK, controls)
}

// Publish handles POST /api/v1/compliance-profiles/{id}/publish.
// Level 3B will implement actual bundle assembly; for now validates
// and returns 501.
func (h *ProfilesHandler) Publish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	// Validate profile has controls.
	controls, err := h.store.GetProfileControls(r.Context(), id)
	if err != nil {
		slog.Error("checking profile controls for publish", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check controls")
		return
	}
	if len(controls) == 0 {
		writeError(w, http.StatusBadRequest, "profile has no controls; add controls before publishing")
		return
	}

	writeError(w, http.StatusNotImplemented, "bundle assembly not yet implemented")
}
