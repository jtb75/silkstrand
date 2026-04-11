package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type TargetHandler struct {
	store store.Store
}

func NewTargetHandler(s store.Store) *TargetHandler {
	return &TargetHandler{store: s}
}

func (h *TargetHandler) List(w http.ResponseWriter, r *http.Request) {
	targets, err := h.store.ListTargets(r.Context())
	if err != nil {
		slog.Error("listing targets", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list targets")
		return
	}
	if targets == nil {
		targets = []model.Target{}
	}
	writeJSON(w, http.StatusOK, targets)
}

func (h *TargetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := h.store.GetTarget(r.Context(), id)
	if err != nil {
		slog.Error("getting target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (h *TargetHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Type == "" || req.Identifier == "" {
		writeError(w, http.StatusBadRequest, "type and identifier are required")
		return
	}

	target, err := h.store.CreateTarget(r.Context(), req)
	if err != nil {
		slog.Error("creating target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create target")
		return
	}
	writeJSON(w, http.StatusCreated, target)
}

func (h *TargetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req model.UpdateTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	target, err := h.store.UpdateTarget(r.Context(), id, req)
	if err != nil {
		slog.Error("updating target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (h *TargetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteTarget(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
