package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

type ScanHandler struct {
	store store.Store
	ps    *pubsub.PubSub
	hub   *websocket.Hub
}

func NewScanHandler(s store.Store, ps *pubsub.PubSub, hub *websocket.Hub) *ScanHandler {
	return &ScanHandler{store: s, ps: ps, hub: hub}
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

	// Publish directive to agent
	if scan.AgentID != nil && h.ps != nil {
		agentID := *scan.AgentID

		if !h.hub.IsConnected(agentID) {
			slog.Warn("agent not connected, scan will wait for agent", "agent_id", agentID, "scan_id", scan.ID)
		}

		directive := pubsub.Directive{
			ScanID:   scan.ID,
			ScanType: scan.ScanType,
			BundleID: scan.BundleID,
		}
		if scan.TargetID != nil {
			directive.TargetID = *scan.TargetID
		}
		if err := h.ps.PublishDirective(r.Context(), agentID, directive); err != nil {
			slog.Error("publishing directive", "agent_id", agentID, "scan_id", scan.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, scan)
}

// Delete removes a scan (and its results via CASCADE) for the caller's
// tenant. Running scans are refused — they need to complete or be failed
// by the agent-disconnect cleanup path first, to avoid orphaning the
// agent's in-flight work.
func (h *ScanHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	scan, err := h.store.GetScan(r.Context(), id)
	if err != nil {
		slog.Error("getting scan for delete", "error", err, "scan_id", id)
		writeError(w, http.StatusInternalServerError, "failed to load scan")
		return
	}
	if scan == nil {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}
	if scan.Status == model.ScanStatusRunning {
		writeError(w, http.StatusConflict,
			"running scans cannot be deleted; wait for completion or agent disconnect")
		return
	}
	if err := h.store.DeleteScan(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "scan not found")
			return
		}
		slog.Error("deleting scan", "error", err, "scan_id", id)
		writeError(w, http.StatusInternalServerError, "failed to delete scan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
