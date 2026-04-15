package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// FindingsHandler serves the ADR 007 D1/D7 `findings` surface. Ingest
// write-through (nuclei hits + bundle compliance results) lives in
// main.go's asset_discovered + scan_results handlers.
type FindingsHandler struct {
	store store.Store
}

func NewFindingsHandler(s store.Store) *FindingsHandler {
	return &FindingsHandler{store: s}
}

func (h *FindingsHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.FindingFilter{
		SourceKind:      q.Get("source_kind"),
		Source:          q.Get("source"),
		Severity:        q.Get("severity"),
		Status:          q.Get("status"),
		AssetEndpointID: q.Get("asset_endpoint_id"),
		CollectionID:    q.Get("collection_id"),
		CVEID:           q.Get("cve_id"),
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = &t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	items, err := h.store.ListFindings(r.Context(), f)
	if err != nil {
		slog.Error("listing findings", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list findings")
		return
	}
	if items == nil {
		items = []model.Finding{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *FindingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, err := h.store.GetFindingByID(r.Context(), id)
	if err != nil {
		slog.Error("getting finding", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if f == nil {
		writeError(w, http.StatusNotFound, "finding not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *FindingsHandler) Suppress(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, model.FindingStatusSuppressed)
}

func (h *FindingsHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	h.setStatus(w, r, model.FindingStatusOpen)
}

func (h *FindingsHandler) setStatus(w http.ResponseWriter, r *http.Request, status string) {
	id := r.PathValue("id")
	if err := h.store.SetFindingStatus(r.Context(), id, status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "finding not found")
			return
		}
		slog.Error("setting finding status", "id", id, "status", status, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update finding")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
