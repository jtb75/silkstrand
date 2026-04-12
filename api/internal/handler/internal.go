package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type InternalHandler struct {
	store   store.Store
	credKey []byte
}

func NewInternalHandler(s store.Store, credKey []byte) *InternalHandler {
	return &InternalHandler{store: s, credKey: credKey}
}

func (h *InternalHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	tenant, err := h.store.CreateTenant(r.Context(), req.Name)
	if err != nil {
		slog.Error("creating tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}
	writeJSON(w, http.StatusCreated, tenant)
}

func (h *InternalHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.store.ListAllTenants(r.Context())
	if err != nil {
		slog.Error("listing tenants", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	if tenants == nil {
		tenants = []model.Tenant{}
	}
	writeJSON(w, http.StatusOK, tenants)
}

func (h *InternalHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tenant, err := h.store.GetTenantByID(r.Context(), id)
	if err != nil {
		slog.Error("getting tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

func (h *InternalHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		if err := h.store.UpdateTenantName(r.Context(), id, *req.Name); err != nil {
			slog.Error("updating tenant name", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update tenant")
			return
		}
	}
	if req.Status != nil {
		if err := h.store.UpdateTenantStatus(r.Context(), id, *req.Status); err != nil {
			slog.Error("updating tenant status", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update tenant")
			return
		}
	}
	if req.Config != nil {
		if err := h.store.UpdateTenantConfig(r.Context(), id, req.Config); err != nil {
			slog.Error("updating tenant config", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update tenant")
			return
		}
	}

	tenant, err := h.store.GetTenantByID(r.Context(), id)
	if err != nil {
		slog.Error("getting updated tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

func (h *InternalHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.UpdateTenantStatus(r.Context(), id, model.TenantStatusInactive); err != nil {
		slog.Error("deactivating tenant", "error", err)
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *InternalHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.ListAllAgents(r.Context())
	if err != nil {
		slog.Error("listing agents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	if agents == nil {
		agents = []model.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *InternalHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.GetDCStats(r.Context())
	if err != nil {
		slog.Error("getting stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// CreateCredential stores an encrypted credential for a target.
func (h *InternalHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string          `json:"tenant_id"`
		TargetID string          `json:"target_id"`
		Type     string          `json:"type"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TenantID == "" || req.TargetID == "" || req.Type == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "tenant_id, target_id, type, and data are required")
		return
	}

	var dataToStore []byte
	if len(h.credKey) > 0 {
		encrypted, err := crypto.Encrypt(req.Data, h.credKey)
		if err != nil {
			slog.Error("encrypting credential", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to encrypt credential")
			return
		}
		dataToStore = encrypted
	} else {
		dataToStore = req.Data
	}

	id, err := h.store.CreateCredential(r.Context(), req.TenantID, req.TargetID, req.Type, dataToStore)
	if err != nil {
		slog.Error("creating credential", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create credential")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}
