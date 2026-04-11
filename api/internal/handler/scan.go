package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

type ScanHandler struct {
	store store.Store
}

func NewScanHandler(s store.Store) *ScanHandler {
	return &ScanHandler{store: s}
}

func (h *ScanHandler) List(w http.ResponseWriter, r *http.Request) {
	scans, err := h.store.ListScans(r.Context())
	if err != nil {
		slog.Error("listing scans", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list scans")
		return
	}
	if scans == nil {
		scans = []model.Scan{}
	}
	writeJSON(w, http.StatusOK, scans)
}

func (h *ScanHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	scan, err := h.store.GetScan(r.Context(), id)
	if err != nil {
		slog.Error("getting scan", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get scan")
		return
	}
	if scan == nil {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}

	// Attach results
	results, err := h.store.GetScanResults(r.Context(), id)
	if err != nil {
		slog.Error("getting scan results", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get scan results")
		return
	}
	scan.Results = results

	// Compute summary
	if len(results) > 0 {
		summary := &model.ScanSummary{Total: len(results)}
		for _, r := range results {
			switch r.Status {
			case "PASS":
				summary.Pass++
			case "FAIL":
				summary.Fail++
			case "ERROR":
				summary.Error++
			case "NOT_APPLICABLE":
				summary.NotApplicable++
			}
		}
		scan.Summary = summary
	}

	writeJSON(w, http.StatusOK, scan)
}

func (h *ScanHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetID == "" || req.BundleID == "" {
		writeError(w, http.StatusBadRequest, "target_id and bundle_id are required")
		return
	}

	scan, err := h.store.CreateScan(r.Context(), req)
	if err != nil {
		slog.Error("creating scan", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create scan")
		return
	}

	// TODO: publish directive to agent via Redis pub/sub

	writeJSON(w, http.StatusCreated, scan)
}
