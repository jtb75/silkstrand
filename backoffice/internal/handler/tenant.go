package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/backoffice/internal/dcclient"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
)

type TenantHandler struct {
	store  store.Store
	dc     *dcclient.Client
	encKey []byte
}

func NewTenantHandler(s store.Store, dc *dcclient.Client, encKey []byte) *TenantHandler {
	return &TenantHandler{store: s, dc: dc, encKey: encKey}
}

func (h *TenantHandler) List(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.store.ListTenants(r.Context())
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

func (h *TenantHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tenant, err := h.store.GetTenant(r.Context(), id)
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

func (h *TenantHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.DataCenterID == "" {
		writeError(w, http.StatusBadRequest, "name and data_center_id are required")
		return
	}

	// Look up the data center
	dc, err := h.store.GetDataCenter(r.Context(), req.DataCenterID)
	if err != nil {
		slog.Error("looking up data center", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up data center")
		return
	}
	if dc == nil {
		writeError(w, http.StatusBadRequest, "data center not found")
		return
	}

	// Create tenant record locally
	tenant, err := h.store.CreateTenant(r.Context(), model.Tenant{
		DataCenterID: req.DataCenterID,
		Name:         req.Name,
		Config:       req.Config,
	})
	if err != nil {
		slog.Error("creating tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create tenant")
		return
	}

	// Two-phase provisioning: try to create in DC
	if len(h.encKey) > 0 {
		conn, err := dcConnFromRecord(dc, h.encKey)
		if err != nil {
			slog.Error("decrypting DC API key for tenant provisioning", "error", err)
			if err := h.store.UpdateTenantProvisioning(r.Context(), tenant.ID, model.ProvisioningFailed, nil); err != nil {
				slog.Error("updating tenant provisioning to failed", "error", err)
			}
			// Return 202 to indicate local creation succeeded but DC provisioning failed
			tenant.ProvisioningStatus = model.ProvisioningFailed
			writeJSON(w, http.StatusAccepted, tenant)
			return
		}

		dcTenant, err := h.dc.CreateTenant(*conn, req.Name)
		if err != nil {
			slog.Error("provisioning tenant in DC", "dc_id", dc.ID, "error", err)
			if err := h.store.UpdateTenantProvisioning(r.Context(), tenant.ID, model.ProvisioningFailed, nil); err != nil {
				slog.Error("updating tenant provisioning to failed", "error", err)
			}
			tenant.ProvisioningStatus = model.ProvisioningFailed
			writeJSON(w, http.StatusAccepted, tenant)
			return
		}

		if err := h.store.UpdateTenantProvisioning(r.Context(), tenant.ID, model.ProvisioningProvisioned, &dcTenant.ID); err != nil {
			slog.Error("updating tenant provisioning to provisioned", "error", err)
		}
		tenant.ProvisioningStatus = model.ProvisioningProvisioned
		tenant.DCTenantID = &dcTenant.ID
	}

	writeJSON(w, http.StatusCreated, tenant)
}

func (h *TenantHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenant, err := h.store.UpdateTenant(r.Context(), id, req.Name, req.Config)
	if err != nil {
		slog.Error("updating tenant", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update tenant")
		return
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	writeJSON(w, http.StatusOK, tenant)
}

func (h *TenantHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTenantStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	// Look up the tenant to get the DC tenant ID and DC ID
	tenant, err := h.store.GetTenant(r.Context(), id)
	if err != nil {
		slog.Error("getting tenant for status update", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	// Sync status to DC if provisioned
	if tenant.DCTenantID != nil && len(h.encKey) > 0 {
		dc, err := h.store.GetDataCenter(r.Context(), tenant.DataCenterID)
		if err != nil {
			slog.Error("looking up data center for status sync", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to look up data center")
			return
		}
		if dc != nil {
			conn, err := dcConnFromRecord(dc, h.encKey)
			if err != nil {
				slog.Error("decrypting DC API key for status sync", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to sync status to data center")
				return
			}
			if err := h.dc.UpdateTenant(*conn, *tenant.DCTenantID, req.Status); err != nil {
				slog.Error("syncing tenant status to DC", "dc_id", dc.ID, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to sync status to data center")
				return
			}
		}
	}

	if err := h.store.UpdateTenantStatus(r.Context(), id, req.Status); err != nil {
		slog.Error("updating tenant status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update tenant status")
		return
	}

	tenant.Status = req.Status
	writeJSON(w, http.StatusOK, tenant)
}

func (h *TenantHandler) Retry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	tenant, err := h.store.GetTenant(r.Context(), id)
	if err != nil {
		slog.Error("getting tenant for retry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get tenant")
		return
	}
	if tenant == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	if tenant.ProvisioningStatus != model.ProvisioningFailed {
		writeError(w, http.StatusBadRequest, "tenant is not in failed provisioning state")
		return
	}

	dc, err := h.store.GetDataCenter(r.Context(), tenant.DataCenterID)
	if err != nil {
		slog.Error("looking up data center for retry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up data center")
		return
	}
	if dc == nil {
		writeError(w, http.StatusInternalServerError, "data center not found")
		return
	}

	if len(h.encKey) == 0 {
		writeError(w, http.StatusInternalServerError, "encryption key not configured")
		return
	}

	conn, err := dcConnFromRecord(dc, h.encKey)
	if err != nil {
		slog.Error("decrypting DC API key for retry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to decrypt API key")
		return
	}

	dcTenant, err := h.dc.CreateTenant(*conn, tenant.Name)
	if err != nil {
		slog.Error("retrying tenant provisioning in DC", "dc_id", dc.ID, "error", err)
		writeError(w, http.StatusBadGateway, "failed to provision tenant in data center")
		return
	}

	if err := h.store.UpdateTenantProvisioning(r.Context(), tenant.ID, model.ProvisioningProvisioned, &dcTenant.ID); err != nil {
		slog.Error("updating tenant provisioning after retry", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update provisioning status")
		return
	}

	tenant.ProvisioningStatus = model.ProvisioningProvisioned
	tenant.DCTenantID = &dcTenant.ID
	writeJSON(w, http.StatusOK, tenant)
}
