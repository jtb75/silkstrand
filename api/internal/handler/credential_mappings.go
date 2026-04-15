package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// CredentialMappingsHandler serves the collection ↔ credential_source
// bindings (ADR 006 roadmap P6). Scan-definitions running against a
// collection resolve creds per endpoint via these rows. v1 only allows
// mappings to `scope = endpoint` collections; asset/finding scopes make
// no sense here because creds bind to endpoints.
type CredentialMappingsHandler struct {
	store store.Store
}

func NewCredentialMappingsHandler(s store.Store) *CredentialMappingsHandler {
	return &CredentialMappingsHandler{store: s}
}

type createMappingRequest struct {
	CollectionID       string `json:"collection_id"`
	CredentialSourceID string `json:"credential_source_id"`
}

type bulkCreateMappingRequest struct {
	CollectionIDs      []string `json:"collection_ids"`
	CredentialSourceID string   `json:"credential_source_id"`
}

type bulkMappingRowResult struct {
	CollectionID string `json:"collection_id"`
	MappingID    string `json:"mapping_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (h *CredentialMappingsHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.store.ListCredentialMappings(r.Context(), claims.TenantID)
	if err != nil {
		slog.Error("listing credential mappings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list credential mappings")
		return
	}
	if items == nil {
		items = []model.CredentialMapping{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *CredentialMappingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := h.store.GetCredentialMapping(r.Context(), id)
	if err != nil {
		slog.Error("getting credential mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "credential mapping not found")
		return
	}
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" || m.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "credential mapping not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *CredentialMappingsHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CollectionID == "" || req.CredentialSourceID == "" {
		writeError(w, http.StatusBadRequest, "collection_id and credential_source_id are required")
		return
	}
	if !h.validate(r, w, claims.TenantID, req.CollectionID, req.CredentialSourceID) {
		return
	}
	m, err := h.store.CreateCredentialMapping(r.Context(), claims.TenantID, req.CollectionID, req.CredentialSourceID)
	if err != nil {
		slog.Error("creating credential mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create credential mapping")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// BulkCreate accepts a list of collection_ids + a single credential_source_id
// and fans out individual mappings. Partial failures are reported per row;
// success is 200 with a report so the UI can show per-row status.
func (h *CredentialMappingsHandler) BulkCreate(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req bulkCreateMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CredentialSourceID == "" || len(req.CollectionIDs) == 0 {
		writeError(w, http.StatusBadRequest, "credential_source_id and collection_ids are required")
		return
	}
	// Validate credential source once up front.
	cs, err := h.store.GetCredentialSource(r.Context(), req.CredentialSourceID)
	if err != nil || cs == nil || cs.TenantID != claims.TenantID {
		writeError(w, http.StatusBadRequest, "credential source not found")
		return
	}
	if cs.Type != model.CredentialSourceTypeStatic &&
		cs.Type != model.CredentialSourceTypeAWSSecretsManager &&
		cs.Type != model.CredentialSourceTypeHashiCorpVault &&
		cs.Type != model.CredentialSourceTypeCyberArk {
		writeError(w, http.StatusBadRequest, "credential source type not mappable to a collection")
		return
	}
	out := make([]bulkMappingRowResult, 0, len(req.CollectionIDs))
	for _, cid := range req.CollectionIDs {
		rr := bulkMappingRowResult{CollectionID: cid}
		if !h.validateCollection(r, claims.TenantID, cid, &rr) {
			out = append(out, rr)
			continue
		}
		m, err := h.store.CreateCredentialMapping(r.Context(), claims.TenantID, cid, req.CredentialSourceID)
		if err != nil {
			slog.Warn("bulk credential mapping row", "collection_id", cid, "error", err)
			rr.Error = err.Error()
		} else {
			rr.MappingID = m.ID
		}
		out = append(out, rr)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (h *CredentialMappingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
	// Verify tenant scope.
	existing, err := h.store.GetCredentialMapping(r.Context(), id)
	if err != nil {
		slog.Error("getting mapping for delete", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if existing == nil || existing.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "credential mapping not found")
		return
	}
	if err := h.store.DeleteCredentialMapping(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "credential mapping not found")
			return
		}
		slog.Error("deleting credential mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete credential mapping")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validate checks both collection + credential source in tenant scope and
// enforces the `scope = endpoint` constraint on the collection.
func (h *CredentialMappingsHandler) validate(r *http.Request, w http.ResponseWriter, tenantID, collectionID, sourceID string) bool {
	c, err := h.store.GetCollection(r.Context(), collectionID)
	if err != nil {
		slog.Error("looking up collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return false
	}
	if c == nil {
		writeError(w, http.StatusBadRequest, "collection not found")
		return false
	}
	if c.Scope != model.CollectionScopeEndpoint {
		writeError(w, http.StatusBadRequest, "credential mappings are only valid for endpoint-scoped collections")
		return false
	}
	cs, err := h.store.GetCredentialSource(r.Context(), sourceID)
	if err != nil {
		slog.Error("looking up credential source", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return false
	}
	if cs == nil || cs.TenantID != tenantID {
		writeError(w, http.StatusBadRequest, "credential source not found")
		return false
	}
	return true
}

// validateCollection is the per-row variant for BulkCreate — records the
// error onto the row result struct instead of writing to the response.
func (h *CredentialMappingsHandler) validateCollection(r *http.Request, _ string, collectionID string, rr *bulkMappingRowResult) bool {
	c, err := h.store.GetCollection(r.Context(), collectionID)
	if err != nil {
		rr.Error = "lookup failed"
		return false
	}
	if c == nil {
		rr.Error = "collection not found"
		return false
	}
	if c.Scope != model.CollectionScopeEndpoint {
		rr.Error = "collection must be endpoint-scoped"
		return false
	}
	return true
}
