package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jtb75/silkstrand/api/internal/audit"
	"github.com/jtb75/silkstrand/api/internal/middleware"
)

// AuditHandler serves the GET /api/v1/audit-events endpoint (ADR 005 D5).
type AuditHandler struct {
	db *sql.DB
}

func NewAuditHandler(db *sql.DB) *AuditHandler {
	return &AuditHandler{db: db}
}

// List returns audit events for the caller's tenant with optional filters.
//
// Query params:
//   - event_type: filter by event type (e.g. "credential.fetch")
//   - actor_id: filter by actor UUID
//   - resource_id: filter by resource UUID
//   - resource_type: filter by resource type (e.g. "target", "agent")
//   - since: RFC3339 lower bound (default 7 days ago)
//   - until: RFC3339 upper bound
//   - limit: max items (default 50, max 200)
//   - cursor: opaque pagination cursor
func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	f := audit.ListFilter{
		TenantID:     claims.TenantID,
		EventType:    q.Get("event_type"),
		ActorID:      q.Get("actor_id"),
		ResourceID:   q.Get("resource_id"),
		ResourceType: q.Get("resource_type"),
	}

	// Parse since (default 7 days ago).
	if s := q.Get("since"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since: must be RFC3339")
			return
		}
		f.Since = &t
	} else {
		t := time.Now().Add(-7 * 24 * time.Hour)
		f.Since = &t
	}

	if s := q.Get("until"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid until: must be RFC3339")
			return
		}
		f.Until = &t
	}

	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		if n > 200 {
			n = 200
		}
		f.Limit = n
	}

	if cursor := q.Get("cursor"); cursor != "" {
		ct, cid, err := audit.DecodeCursor(cursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		f.CursorTime = ct
		f.CursorID = cid
	}

	result, err := audit.ListAuditEvents(r.Context(), h.db, f)
	if err != nil {
		slog.Error("listing audit events", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list audit events")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
