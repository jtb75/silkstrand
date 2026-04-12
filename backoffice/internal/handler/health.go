package handler

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/jtb75/silkstrand/backoffice/internal/dcclient"
	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
)

type HealthHandler struct {
	store  store.Store
	dc     *dcclient.Client
	encKey []byte
}

func NewHealthHandler(s store.Store, dc *dcclient.Client, encKey []byte) *HealthHandler {
	return &HealthHandler{store: s, dc: dc, encKey: encKey}
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		slog.Error("readiness check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HealthHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	dcs, err := h.store.ListDataCenters(r.Context())
	if err != nil {
		slog.Error("listing data centers for dashboard", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list data centers")
		return
	}

	var wg sync.WaitGroup
	results := make([]model.DataCenterStats, len(dcs))

	for i, dc := range dcs {
		wg.Add(1)
		go func(idx int, dc model.DataCenter) {
			defer wg.Done()
			results[idx].DataCenter = dc

			if dc.Status != model.DCStatusActive || len(h.encKey) == 0 {
				return
			}

			conn, err := dcConnFromRecord(&dc, h.encKey)
			if err != nil {
				results[idx].Error = "failed to decrypt API key"
				return
			}

			stats, err := h.dc.GetStats(*conn)
			if err != nil {
				results[idx].Error = err.Error()
				return
			}
			results[idx].Stats = stats
		}(i, dc)
	}

	wg.Wait()

	writeJSON(w, http.StatusOK, model.DashboardStats{DataCenters: results})
}
