package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/store"
)

type HealthHandler struct {
	store     store.Store
	redisPing func(ctx context.Context) error
}

func NewHealthHandler(s store.Store, redisPing func(ctx context.Context) error) *HealthHandler {
	return &HealthHandler{store: s, redisPing: redisPing}
}

// Healthz is a basic liveness check — always returns 200 if the process is running.
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz checks connectivity to Postgres and Redis.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := map[string]string{
		"status":   "ok",
		"postgres": "ok",
		"redis":    "ok",
	}
	httpStatus := http.StatusOK

	if err := h.store.Ping(ctx); err != nil {
		status["postgres"] = err.Error()
		status["status"] = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	if h.redisPing != nil {
		if err := h.redisPing(ctx); err != nil {
			status["redis"] = err.Error()
			status["status"] = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(status)
}
