package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// CollectionsHandler serves the ADR 006 D5 `collections` CRUD surface.
// Scopes are asset / endpoint / finding; the predicate evaluator's
// scope-aware dispatcher lands in P4, so in P1 we accept + persist
// arbitrary predicates without validating them beyond JSON-wellformedness.
type CollectionsHandler struct {
	store store.Store
}

func NewCollectionsHandler(s store.Store) *CollectionsHandler {
	return &CollectionsHandler{store: s}
}

type createCollectionRequest struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Scope             string          `json:"scope"`
	Predicate         json.RawMessage `json:"predicate"`
	IsDashboardWidget bool            `json:"is_dashboard_widget"`
	WidgetKind        string          `json:"widget_kind"`
}

func (h *CollectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListCollections(r.Context())
	if err != nil {
		slog.Error("listing collections", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}
	if items == nil {
		items = []model.Collection{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *CollectionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := h.store.GetCollection(r.Context(), id)
	if err != nil {
		slog.Error("getting collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *CollectionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Scope == "" {
		req.Scope = model.CollectionScopeEndpoint
	}
	switch req.Scope {
	case model.CollectionScopeAsset, model.CollectionScopeEndpoint, model.CollectionScopeFinding:
	default:
		writeError(w, http.StatusBadRequest, "invalid scope: must be asset | endpoint | finding")
		return
	}
	var desc *string
	if req.Description != "" {
		d := req.Description
		desc = &d
	}
	var widget *string
	if req.WidgetKind != "" {
		w := req.WidgetKind
		widget = &w
	}
	c := model.Collection{
		Name:              req.Name,
		Description:       desc,
		Scope:             req.Scope,
		Predicate:         req.Predicate,
		IsDashboardWidget: req.IsDashboardWidget,
		WidgetKind:        widget,
	}
	out, err := h.store.CreateCollection(r.Context(), c)
	if err != nil {
		slog.Error("creating collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create collection")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *CollectionsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := h.store.GetCollection(r.Context(), id)
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	var req createCollectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Scope != "" {
		existing.Scope = req.Scope
	}
	if len(req.Predicate) > 0 {
		existing.Predicate = req.Predicate
	}
	if req.Description != "" {
		d := req.Description
		existing.Description = &d
	}
	if req.WidgetKind != "" {
		wk := req.WidgetKind
		existing.WidgetKind = &wk
	}
	existing.IsDashboardWidget = req.IsDashboardWidget
	out, err := h.store.UpdateCollection(r.Context(), *existing)
	if err != nil {
		slog.Error("updating collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update collection")
		return
	}
	if out == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CollectionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteCollection(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "collection not found")
			return
		}
		slog.Error("deleting collection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete collection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Preview is the ad-hoc predicate evaluator endpoint from the old
// asset-sets surface. P4 wires the scope-aware evaluator; for now it
// returns 501 so the UI can feature-flag accordingly.
func (h *CollectionsHandler) Preview(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "collection preview lands in P4 with the scope-aware predicate evaluator")
}
