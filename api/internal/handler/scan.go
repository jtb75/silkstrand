package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/events"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

// ScanHandler serves scan execution history. Per ADR 007 D3 the
// authoritative scan configuration surface is `scan_definitions`; scans
// rows remain execution history pointing back at a definition (nullable
// for the ad-hoc debug path). Results are gone in favor of the
// `findings` table (P3). Get therefore no longer attaches results.
type ScanHandler struct {
	store store.Store
	ps    *pubsub.PubSub
	hub   *websocket.Hub
	bus   events.Bus
}

func NewScanHandler(s store.Store, ps *pubsub.PubSub, hub *websocket.Hub, bus events.Bus) *ScanHandler {
	return &ScanHandler{store: s, ps: ps, hub: hub, bus: bus}
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
	if scan.AgentID != nil && h.ps != nil {
		agentID := *scan.AgentID
		// Check if agent already has a running/pending scan — queue if busy.
		busy, busyErr := h.store.AgentHasRunningScanExcluding(r.Context(), agentID, scan.ID)
		if busyErr != nil {
			slog.Error("checking agent busy", "agent_id", agentID, "error", busyErr)
		}
		if busy {
			if err := h.store.UpdateScanStatus(r.Context(), scan.ID, model.ScanStatusQueued); err != nil {
				slog.Error("queueing scan", "scan_id", scan.ID, "error", err)
			} else {
				scan.Status = model.ScanStatusQueued
				slog.Info("scan queued", "scan_id", scan.ID, "agent_id", agentID)
			}
		} else {
			if !h.hub.IsConnected(agentID) {
				slog.Warn("agent not connected, scan will wait for agent", "agent_id", agentID, "scan_id", scan.ID)
			}
			bundleID := ""
			if scan.BundleID != nil {
				bundleID = *scan.BundleID
			}
			directive := pubsub.Directive{
				ScanID:   scan.ID,
				ScanType: scan.ScanType,
				BundleID: bundleID,
			}
			if scan.TargetID != nil {
				directive.TargetID = *scan.TargetID
			}
			if err := h.ps.PublishDirective(r.Context(), agentID, directive); err != nil {
				slog.Error("publishing directive", "agent_id", agentID, "scan_id", scan.ID, "error", err)
			}
		}
	}
	publishScanStatus(r.Context(), h.bus, scan)
	writeJSON(w, http.StatusCreated, scan)
}

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

// publishScanStatus emits a scan_status event on the bus so SSE
// subscribers (toast notifications, cache invalidation) get real-time
// scan state changes. Non-blocking; failures are logged and swallowed.
func publishScanStatus(ctx context.Context, bus events.Bus, scan *model.Scan) {
	if bus == nil || scan == nil {
		return
	}
	type scanStatusPayload struct {
		Status           string  `json:"status"`
		ScanDefinitionID *string `json:"scan_definition_id"`
		AgentID          *string `json:"agent_id"`
	}
	payload, _ := json.Marshal(scanStatusPayload{
		Status:           scan.Status,
		ScanDefinitionID: scan.ScanDefinitionID,
		AgentID:          scan.AgentID,
	})
	if err := bus.Publish(ctx, events.Event{
		TenantID:     scan.TenantID,
		Kind:         "scan_status",
		ResourceType: "scan",
		ResourceID:   scan.ID,
		OccurredAt:   time.Now().UTC(),
		Payload:      payload,
	}); err != nil {
		slog.Warn("scan_status publish failed", "scan_id", scan.ID, "error", err)
	}
}

// PublishScanStatusFromScan is an exported variant so main.go WSS
// handlers can emit scan_status events without duplicating the logic.
func PublishScanStatusFromScan(ctx context.Context, bus events.Bus, scan *model.Scan) {
	publishScanStatus(ctx, bus, scan)
}
