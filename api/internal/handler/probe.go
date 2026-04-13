package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

// ProbeHandler serves the "Test connection" button on the Targets page.
// It sends a probe directive to the target's agent over WSS, waits for the
// agent's reply, and returns the result synchronously.
type ProbeHandler struct {
	store  store.Store
	hub    *websocket.Hub
	encKey []byte
}

func NewProbeHandler(s store.Store, hub *websocket.Hub, encKey []byte) *ProbeHandler {
	return &ProbeHandler{store: s, hub: hub, encKey: encKey}
}

const probeTimeout = 15 * time.Second

// POST /api/v1/targets/{id}/probe (tenant-authed)
// Returns { ok, error?, detail? }.
func (h *ProbeHandler) Probe(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	target, err := h.store.GetTarget(r.Context(), targetID)
	if err != nil {
		slog.Error("probe: getting target", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up target")
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	if target.AgentID == nil || *target.AgentID == "" {
		writeError(w, http.StatusBadRequest, "target has no agent assigned")
		return
	}

	// Pull + decrypt credential if one exists. OK if missing — probe will
	// surface that in the agent's reply.
	var credsRaw json.RawMessage
	if stored, err := h.store.GetCredentialsByTarget(r.Context(), targetID); err == nil && len(stored) > 0 {
		plaintext, err := decryptIfPossible(stored, h.encKey)
		if err != nil {
			slog.Warn("probe: decrypting credential", "error", err)
			writeError(w, http.StatusInternalServerError, "credential decrypt failed")
			return
		}
		credsRaw = plaintext
	}

	// Correlate request/response with a random probe_id.
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	probeID := hex.EncodeToString(buf)
	ch := h.hub.RegisterProbeWaiter(probeID)
	defer h.hub.ReleaseProbeWaiter(probeID)

	payload, _ := json.Marshal(websocket.ProbePayload{
		ProbeID:          probeID,
		TargetType:       target.Type,
		TargetIdentifier: target.Identifier,
		TargetConfig:     target.Config,
		Credentials:      credsRaw,
	})
	if err := h.hub.Send(*target.AgentID, websocket.Message{
		Type: websocket.TypeProbe, Payload: payload,
	}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "agent not connected")
		return
	}

	select {
	case result := <-ch:
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     result.OK,
			"error":  result.Error,
			"detail": result.Detail,
		})
	case <-time.After(probeTimeout):
		writeError(w, http.StatusGatewayTimeout, "agent did not reply in time")
	}
}

// decryptIfPossible returns the stored bytes as-is when no encryption key is
// configured (dev); otherwise AES-GCM decrypts.
func decryptIfPossible(stored []byte, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return stored, nil
	}
	return crypto.Decrypt(stored, key)
}
