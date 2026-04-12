package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/backoffice/internal/model"
	"github.com/jtb75/silkstrand/backoffice/internal/store"
)

type AuditHandler struct {
	store store.Store
}

func NewAuditHandler(s store.Store) *AuditHandler {
	return &AuditHandler{store: s}
}

// GET /api/v1/audit?tenant_id=&actor_id=&action=&limit=
// Backoffice-admin authed.
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var filter store.AuditFilter
	if v := q.Get("tenant_id"); v != "" {
		filter.TenantID = &v
	}
	if v := q.Get("actor_id"); v != "" {
		filter.ActorID = &v
	}
	if v := q.Get("action"); v != "" {
		filter.Action = &v
	}
	if v := q.Get("limit"); v != "" {
		var n int
		// Best-effort parse; invalid → default limit applied in store.
		_, _ = fmt.Sscanf(v, "%d", &n)
		filter.Limit = n
	}

	entries, err := h.store.ListAuditLog(r.Context(), filter)
	if err != nil {
		slog.Error("listing audit log", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list audit log")
		return
	}
	if entries == nil {
		entries = []model.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
