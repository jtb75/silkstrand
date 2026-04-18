package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/open-policy-agent/opa/v1/rego"
	"gopkg.in/yaml.v3"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// TenantPoliciesHandler serves the tenant policy CRUD surface
// (ADR 011 D9 — tenant policy management).
type TenantPoliciesHandler struct {
	store      store.Store
	policiesDir string // path to policies/ directory for copy-from-builtin
}

func NewTenantPoliciesHandler(s store.Store, policiesDir string) *TenantPoliciesHandler {
	return &TenantPoliciesHandler{store: s, policiesDir: policiesDir}
}

type createTenantPolicyRequest struct {
	ControlID   string          `json:"control_id"`
	Origin      string          `json:"origin"`
	BasedOn     *string         `json:"based_on,omitempty"`
	Name        string          `json:"name"`
	Severity    string          `json:"severity"`
	RegoSource  string          `json:"rego_source"`
	CollectorID string          `json:"collector_id"`
	FactKeys    json.RawMessage `json:"fact_keys"`
	Frameworks  json.RawMessage `json:"frameworks"`
	Tags        json.RawMessage `json:"tags"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

type updateTenantPolicyRequest struct {
	ControlID   *string         `json:"control_id,omitempty"`
	Name        *string         `json:"name,omitempty"`
	Severity    *string         `json:"severity,omitempty"`
	RegoSource  *string         `json:"rego_source,omitempty"`
	CollectorID *string         `json:"collector_id,omitempty"`
	FactKeys    json.RawMessage `json:"fact_keys,omitempty"`
	Frameworks  json.RawMessage `json:"frameworks,omitempty"`
	Tags        json.RawMessage `json:"tags,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
}

type validateRegoRequest struct {
	RegoSource string `json:"rego_source"`
}

type validateRegoResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// policyMetadata mirrors the policy.yaml structure on disk.
type policyMetadata struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Severity  string `yaml:"severity"`
	Collector string `yaml:"collector"`
	FactKeys  []string `yaml:"fact_keys"`
	Engine    []struct {
		Name     string   `yaml:"name"`
		Versions []string `yaml:"versions"`
	} `yaml:"engine"`
	Frameworks []struct {
		ID      string `yaml:"id"`
		Section string `yaml:"section"`
		Title   string `yaml:"title"`
	} `yaml:"frameworks"`
	Tags []string `yaml:"tags"`
}

// List handles GET /api/v1/tenant-policies.
func (h *TenantPoliciesHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListTenantPolicies(r.Context())
	if err != nil {
		slog.Error("listing tenant policies", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tenant policies")
		return
	}
	if items == nil {
		items = []model.TenantPolicy{}
	}
	writeJSON(w, http.StatusOK, items)
}

// Get handles GET /api/v1/tenant-policies/{id}.
func (h *TenantPoliciesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.store.GetTenantPolicy(r.Context(), id)
	if err != nil {
		slog.Error("getting tenant policy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant policy")
		return
	}
	if p == nil {
		writeError(w, http.StatusNotFound, "tenant policy not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// Create handles POST /api/v1/tenant-policies.
func (h *TenantPoliciesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createTenantPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ControlID == "" || req.Name == "" || req.RegoSource == "" || req.CollectorID == "" || req.Severity == "" {
		writeError(w, http.StatusBadRequest, "control_id, name, severity, rego_source, and collector_id are required")
		return
	}
	if req.Origin == "" {
		req.Origin = model.TenantPolicyOriginCustom
	}
	if req.Origin != model.TenantPolicyOriginDerived && req.Origin != model.TenantPolicyOriginCustom {
		writeError(w, http.StatusBadRequest, "origin must be 'derived' or 'custom'")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	p := model.TenantPolicy{
		ControlID:   req.ControlID,
		Origin:      req.Origin,
		BasedOn:     req.BasedOn,
		Name:        req.Name,
		Severity:    req.Severity,
		RegoSource:  req.RegoSource,
		CollectorID: req.CollectorID,
		FactKeys:    defaultJSONArray(req.FactKeys),
		Frameworks:  defaultJSONArray(req.Frameworks),
		Tags:        defaultJSONArray(req.Tags),
		Enabled:     enabled,
	}

	out, err := h.store.CreateTenantPolicy(r.Context(), p)
	if err != nil {
		slog.Error("creating tenant policy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create tenant policy")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// Update handles PUT /api/v1/tenant-policies/{id}.
func (h *TenantPoliciesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetTenantPolicy(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "tenant policy not found")
		return
	}

	var req updateTenantPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updated := *existing
	if req.ControlID != nil {
		updated.ControlID = *req.ControlID
	}
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Severity != nil {
		updated.Severity = *req.Severity
	}
	if req.RegoSource != nil {
		updated.RegoSource = *req.RegoSource
	}
	if req.CollectorID != nil {
		updated.CollectorID = *req.CollectorID
	}
	if req.FactKeys != nil {
		updated.FactKeys = req.FactKeys
	}
	if req.Frameworks != nil {
		updated.Frameworks = req.Frameworks
	}
	if req.Tags != nil {
		updated.Tags = req.Tags
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}

	out, err := h.store.UpdateTenantPolicy(r.Context(), id, updated)
	if err != nil {
		slog.Error("updating tenant policy", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update tenant policy")
		return
	}
	if out == nil {
		writeError(w, http.StatusNotFound, "tenant policy not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Delete handles DELETE /api/v1/tenant-policies/{id}.
func (h *TenantPoliciesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteTenantPolicy(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "tenant policy not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Validate handles POST /api/v1/tenant-policies/validate.
// Compile-checks Rego source without saving.
func (h *TenantPoliciesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRegoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RegoSource == "" {
		writeError(w, http.StatusBadRequest, "rego_source is required")
		return
	}

	_, err := rego.New(
		rego.Query("data"),
		rego.Module("validate.rego", req.RegoSource),
	).PrepareForEval(context.Background())

	if err != nil {
		writeJSON(w, http.StatusOK, validateRegoResponse{Valid: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, validateRegoResponse{Valid: true})
}

// CopyFromBuiltin handles POST /api/v1/tenant-policies/copy-from/{builtin_control_id}.
// Reads the builtin policy from the policies directory and creates a tenant
// policy with origin='derived'.
func (h *TenantPoliciesHandler) CopyFromBuiltin(w http.ResponseWriter, r *http.Request) {
	builtinID := r.PathValue("builtin_control_id")
	if builtinID == "" {
		writeError(w, http.StatusBadRequest, "builtin_control_id is required")
		return
	}

	policyDir := filepath.Join(h.policiesDir, builtinID)
	regoPath := filepath.Join(policyDir, "policy.rego")
	yamlPath := filepath.Join(policyDir, "policy.yaml")

	regoData, err := os.ReadFile(regoPath)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("builtin policy %q not found", builtinID))
		return
	}

	// Parse metadata from policy.yaml.
	var meta policyMetadata
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		// policy.yaml missing — use reasonable defaults.
		meta = policyMetadata{
			ID:       builtinID,
			Name:     builtinID,
			Severity: "medium",
		}
	} else {
		if err := yaml.Unmarshal(yamlData, &meta); err != nil {
			slog.Warn("failed to parse policy.yaml", "control_id", builtinID, "error", err)
			meta = policyMetadata{ID: builtinID, Name: builtinID, Severity: "medium"}
		}
	}

	factKeysJSON, _ := json.Marshal(meta.FactKeys)
	if meta.FactKeys == nil {
		factKeysJSON = []byte("[]")
	}

	frameworksJSON, _ := json.Marshal(meta.Frameworks)
	if meta.Frameworks == nil {
		frameworksJSON = []byte("[]")
	}

	tagsJSON, _ := json.Marshal(meta.Tags)
	if meta.Tags == nil {
		tagsJSON = []byte("[]")
	}

	collectorID := meta.Collector
	if collectorID == "" {
		collectorID = "unknown"
	}

	basedOn := builtinID

	p := model.TenantPolicy{
		ControlID:   builtinID,
		Origin:      model.TenantPolicyOriginDerived,
		BasedOn:     &basedOn,
		Name:        meta.Name,
		Severity:    meta.Severity,
		RegoSource:  string(regoData),
		CollectorID: collectorID,
		FactKeys:    json.RawMessage(factKeysJSON),
		Frameworks:  json.RawMessage(frameworksJSON),
		Tags:        json.RawMessage(tagsJSON),
		Enabled:     true,
	}

	out, err := h.store.CreateTenantPolicy(r.Context(), p)
	if err != nil {
		slog.Error("copying builtin policy", "error", err, "builtin_control_id", builtinID)
		writeError(w, http.StatusInternalServerError, "failed to copy builtin policy")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// defaultJSONArray returns the input if non-nil, otherwise []byte("[]").
func defaultJSONArray(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage("[]")
	}
	return v
}
