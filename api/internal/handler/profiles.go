package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jtb75/silkstrand/api/internal/bundler"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// ProfilesHandler serves the compliance profiles CRUD surface
// (ADR 010 D9 — Level 3A) and the server-side bundle assembler (Level 3B).
type ProfilesHandler struct {
	store           store.Store
	controlsDir     string // path to controls/ directory for bundle assembly
	bundleStorePath string // path to write assembled tarballs
}

func NewProfilesHandler(s store.Store, controlsDir, bundleStorePath string) *ProfilesHandler {
	return &ProfilesHandler{
		store:           s,
		controlsDir:     controlsDir,
		bundleStorePath: bundleStorePath,
	}
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
// Assembles a bundle tarball from the profile's controls, stores it,
// registers it as a bundle, and links the profile to the bundle.
func (h *ProfilesHandler) Publish(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	claims := middleware.GetClaims(r.Context())

	existing, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	// Load the profile's control IDs.
	controlIDs, err := h.store.GetProfileControls(r.Context(), id)
	if err != nil {
		slog.Error("loading profile controls for publish", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load controls")
		return
	}
	if len(controlIDs) == 0 {
		writeError(w, http.StatusBadRequest, "profile has no controls; add controls before publishing")
		return
	}

	engine := deriveEngine(h.controlsDir, controlIDs)

	framework := "custom"
	if existing.BaseFramework != nil && *existing.BaseFramework != "" {
		framework = *existing.BaseFramework
	}

	// Re-use the profile's existing bundle_id if re-publishing,
	// otherwise pass empty string so the DB generates a new UUID.
	bundleID := ""
	if existing.BundleID != nil && *existing.BundleID != "" {
		bundleID = *existing.BundleID
	}

	version := fmt.Sprintf("%d.0.0", existing.Version+1)

	// Assemble the bundle tarball.
	result, err := bundler.Build(bundler.BuildOptions{
		BundleID:    bundleID,
		Name:        existing.Name,
		Version:     version,
		Framework:   framework,
		Engine:      engine,
		ControlIDs:  controlIDs,
		ControlsDir: h.controlsDir,
	})
	if err != nil {
		slog.Error("assembling bundle", "error", err, "profile_id", id)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to assemble bundle: %s", err))
		return
	}

	// Store the tarball to disk.
	if h.bundleStorePath != "" {
		if err := storeTarball(h.bundleStorePath, existing.Name, version, result.Tarball); err != nil {
			slog.Warn("failed to store assembled tarball locally", "error", err)
			// Non-fatal: DB registration is the primary job.
		}
	}

	// Upsert the bundle row.
	tenantID := ""
	if claims != nil {
		tenantID = claims.TenantID
	}
	b := model.Bundle{
		ID:           bundleID,
		TenantID:     &tenantID,
		Name:         existing.Name,
		Version:      version,
		Framework:    framework,
		TargetType:   engine,
		Engine:       &engine,
		ControlCount: result.ControlCount,
		TarballHash:  &result.Hash,
	}
	if result.Signature != "" {
		sig := result.Signature
		b.Signature = &sig
	}

	bundle, err := h.store.UpsertBundle(r.Context(), b)
	if err != nil {
		slog.Error("upserting assembled bundle", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save bundle")
		return
	}

	// Build bundle_controls rows from control.yaml files.
	controls := buildControlRowsFromDir(bundle.ID, engine, controlIDs, h.controlsDir)
	if err := h.store.ReplaceBundleControls(r.Context(), bundle.ID, controls); err != nil {
		slog.Error("replacing bundle controls for profile", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save bundle controls")
		return
	}

	// Link the profile to the bundle and bump its version.
	if err := h.store.PublishProfile(r.Context(), id, bundle.ID); err != nil {
		slog.Error("publishing profile", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to publish profile")
		return
	}

	// Re-fetch the profile to return the updated state.
	published, err := h.store.GetComplianceProfile(r.Context(), id)
	if err != nil || published == nil {
		slog.Error("re-fetching published profile", "error", err)
		writeError(w, http.StatusInternalServerError, "published but failed to fetch result")
		return
	}

	slog.Info("profile published",
		"profile_id", id,
		"bundle_id", bundle.ID,
		"version", version,
		"controls", result.ControlCount)

	writeJSON(w, http.StatusOK, published)
}

// deriveEngine reads control IDs to extract the engine name from the
// control ID prefix convention. Falls back to "unknown".
func deriveEngine(controlsDir string, controlIDs []string) string {
	for _, cid := range controlIDs {
		if e := engineFromControlID(cid); e != "" {
			return e
		}
	}
	// Fallback: try parsing control.yaml for the first control.
	for _, cid := range controlIDs {
		path := filepath.Join(controlsDir, cid, "control.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		cm := parseControlYAML(data)
		if cm != nil && cm.ID != "" {
			if e := engineFromControlID(cm.ID); e != "" {
				return e
			}
		}
	}
	return "unknown"
}

// engineFromControlID extracts the engine from a control ID prefix.
// e.g., "pg-tls-enabled" -> "postgresql", "mssql-tde" -> "mssql",
// "mongo-tls-transport-encryption" -> "mongodb".
func engineFromControlID(id string) string {
	switch {
	case strings.HasPrefix(id, "pg-"):
		return "postgresql"
	case strings.HasPrefix(id, "mssql-"):
		return "mssql"
	case strings.HasPrefix(id, "mongo-"):
		return "mongodb"
	case strings.HasPrefix(id, "mysql-"):
		return "mysql"
	default:
		return ""
	}
}

// buildControlRowsFromDir reads control.yaml files from the controls
// directory and builds BundleControl model rows, matching the logic in
// bundles.go Upload handler.
func buildControlRowsFromDir(bundleID, engine string, controlIDs []string, controlsDir string) []model.BundleControl {
	var controls []model.BundleControl
	for _, ctrlID := range controlIDs {
		bc := model.BundleControl{
			BundleID:       bundleID,
			ControlID:      ctrlID,
			Name:           ctrlID,
			Engine:         engine,
			EngineVersions: json.RawMessage(`[]`),
			Tags:           json.RawMessage(`[]`),
		}

		path := filepath.Join(controlsDir, ctrlID, "control.yaml")
		data, err := os.ReadFile(path)
		if err == nil {
			cm := parseControlYAML(data)
			if cm != nil {
				populateControlFromManifest(&bc, cm)
			}
		}

		controls = append(controls, bc)
	}
	return controls
}
