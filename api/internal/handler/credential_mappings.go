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

// CredentialMappingsHandler serves credential_source bindings scoped to
// collections, assets, or individual endpoints. The scheduler and
// forwardDirective resolve credentials with most-specific-first precedence:
// endpoint > asset > collection.
type CredentialMappingsHandler struct {
	store store.Store
}

func NewCredentialMappingsHandler(s store.Store) *CredentialMappingsHandler {
	return &CredentialMappingsHandler{store: s}
}

type createMappingRequest struct {
	ScopeKind          string `json:"scope_kind"`
	CollectionID       string `json:"collection_id"`
	AssetEndpointID    string `json:"asset_endpoint_id"`
	AssetID            string `json:"asset_id"`
	CredentialSourceID string `json:"credential_source_id"`
}

type bulkCreateMappingRequest struct {
	// Legacy: collection-scoped bulk.
	CollectionIDs []string `json:"collection_ids"`
	// New: endpoint-scoped bulk.
	EndpointIDs []string `json:"endpoint_ids"`
	// New: asset-scoped bulk.
	AssetIDs           []string `json:"asset_ids"`
	CredentialSourceID string   `json:"credential_source_id"`
}

type bulkMappingRowResult struct {
	ID       string `json:"id,omitempty"`
	ScopeID  string `json:"scope_id"`
	MappingID string `json:"mapping_id,omitempty"`
	Error    string `json:"error,omitempty"`
	// Legacy compat — still included when scope=collection.
	CollectionID string `json:"collection_id,omitempty"`
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
	if req.CredentialSourceID == "" {
		writeError(w, http.StatusBadRequest, "credential_source_id is required")
		return
	}
	// Default scope_kind to collection for backwards compat.
	if req.ScopeKind == "" {
		req.ScopeKind = model.MappingScopeCollection
	}

	in, err := h.buildInput(r, w, claims.TenantID, req)
	if err != nil {
		return // error already written
	}
	m, err := h.store.CreateCredentialMapping(r.Context(), *in)
	if err != nil {
		slog.Error("creating credential mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create credential mapping")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// BulkCreate fans out mappings. Accepts endpoint_ids (scope=asset_endpoint),
// asset_ids (scope=asset), or collection_ids (scope=collection, legacy).
// Exactly one set should be non-empty.
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
	if req.CredentialSourceID == "" {
		writeError(w, http.StatusBadRequest, "credential_source_id is required")
		return
	}
	// Validate credential source once up front.
	cs, err := h.store.GetCredentialSource(r.Context(), req.CredentialSourceID)
	if err != nil || cs == nil || cs.TenantID != claims.TenantID {
		writeError(w, http.StatusBadRequest, "credential source not found")
		return
	}

	// Determine which scope this bulk call targets.
	type scopeItem struct {
		kind string
		id   string
	}
	var items []scopeItem
	switch {
	case len(req.EndpointIDs) > 0:
		for _, id := range req.EndpointIDs {
			items = append(items, scopeItem{model.MappingScopeAssetEndpoint, id})
		}
	case len(req.AssetIDs) > 0:
		for _, id := range req.AssetIDs {
			items = append(items, scopeItem{model.MappingScopeAsset, id})
		}
	case len(req.CollectionIDs) > 0:
		for _, id := range req.CollectionIDs {
			items = append(items, scopeItem{model.MappingScopeCollection, id})
		}
	default:
		writeError(w, http.StatusBadRequest, "endpoint_ids, asset_ids, or collection_ids required")
		return
	}

	out := make([]bulkMappingRowResult, 0, len(items))
	for _, si := range items {
		rr := bulkMappingRowResult{ScopeID: si.id}
		if si.kind == model.MappingScopeCollection {
			rr.CollectionID = si.id
		}
		in := store.CreateCredentialMappingInput{
			TenantID:           claims.TenantID,
			ScopeKind:          si.kind,
			CredentialSourceID: req.CredentialSourceID,
		}
		switch si.kind {
		case model.MappingScopeAssetEndpoint:
			in.AssetEndpointID = &si.id
		case model.MappingScopeAsset:
			in.AssetID = &si.id
		case model.MappingScopeCollection:
			in.CollectionID = &si.id
		}
		m, err := h.store.CreateCredentialMapping(r.Context(), in)
		if err != nil {
			slog.Warn("bulk credential mapping row", "scope_kind", si.kind, "id", si.id, "error", err)
			rr.Error = err.Error()
		} else {
			rr.MappingID = m.ID
		}
		out = append(out, rr)
	}

	// Return mapped count for endpoint bulk compat with the existing
	// Assets page bulkMapCredentials caller.
	mapped := 0
	for _, rr := range out {
		if rr.MappingID != "" {
			mapped++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": out, "mapped": mapped})
}

func (h *CredentialMappingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := r.PathValue("id")
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

// buildInput validates and constructs a CreateCredentialMappingInput from a
// Create request. On validation failure it writes the error to w and returns
// a non-nil error.
func (h *CredentialMappingsHandler) buildInput(r *http.Request, w http.ResponseWriter, tenantID string, req createMappingRequest) (*store.CreateCredentialMappingInput, error) {
	in := store.CreateCredentialMappingInput{
		TenantID:           tenantID,
		ScopeKind:          req.ScopeKind,
		CredentialSourceID: req.CredentialSourceID,
	}

	switch req.ScopeKind {
	case model.MappingScopeCollection:
		if req.CollectionID == "" {
			writeError(w, http.StatusBadRequest, "collection_id is required for scope_kind=collection")
			return nil, errors.New("validation")
		}
		c, err := h.store.GetCollection(r.Context(), req.CollectionID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed")
			return nil, err
		}
		if c == nil {
			writeError(w, http.StatusBadRequest, "collection not found")
			return nil, errors.New("validation")
		}
		if c.Scope != model.CollectionScopeEndpoint {
			writeError(w, http.StatusBadRequest, "credential mappings are only valid for endpoint-scoped collections")
			return nil, errors.New("validation")
		}
		in.CollectionID = &req.CollectionID

	case model.MappingScopeAssetEndpoint:
		if req.AssetEndpointID == "" {
			writeError(w, http.StatusBadRequest, "asset_endpoint_id is required for scope_kind=asset_endpoint")
			return nil, errors.New("validation")
		}
		in.AssetEndpointID = &req.AssetEndpointID

	case model.MappingScopeAsset:
		if req.AssetID == "" {
			writeError(w, http.StatusBadRequest, "asset_id is required for scope_kind=asset")
			return nil, errors.New("validation")
		}
		in.AssetID = &req.AssetID

	default:
		writeError(w, http.StatusBadRequest, "scope_kind must be collection, asset_endpoint, or asset")
		return nil, errors.New("validation")
	}

	// Validate credential source belongs to this tenant.
	cs, err := h.store.GetCredentialSource(r.Context(), req.CredentialSourceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return nil, err
	}
	if cs == nil || cs.TenantID != tenantID {
		writeError(w, http.StatusBadRequest, "credential source not found")
		return nil, errors.New("validation")
	}

	return &in, nil
}
