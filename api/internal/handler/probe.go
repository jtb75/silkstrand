package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

// ProbeHandler serves the "Test connection" button on the Targets page.
// It sends a probe request via Redis pub/sub (so it can hop instances)
// and waits on the per-probe-id result channel for the agent's reply.
type ProbeHandler struct {
	store  store.Store
	ps     *pubsub.PubSub
	encKey []byte
}

func NewProbeHandler(s store.Store, ps *pubsub.PubSub, encKey []byte) *ProbeHandler {
	return &ProbeHandler{store: s, ps: ps, encKey: encKey}
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

	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	probeID := hex.EncodeToString(buf)

	probePayload, _ := json.Marshal(websocket.ProbePayload{
		ProbeID:          probeID,
		TargetType:       target.Type,
		TargetIdentifier: target.Identifier,
		TargetConfig:     target.Config,
		Credentials:      credsRaw,
	})

	// Subscribe to result channel BEFORE publishing — pub/sub has no
	// durability and the agent's reply could land before a late
	// subscriber connects.
	resultBytes, err := h.ps.AwaitProbeResult(r.Context(), probeID, probeTimeout, func() error {
		return h.ps.PublishProbe(r.Context(), *target.AgentID, probePayload)
	})
	if err != nil {
		if errors.Is(err, r.Context().Err()) {
			writeError(w, http.StatusServiceUnavailable, "request cancelled")
			return
		}
		// Distinguish timeout from other errors for the UI.
		if err.Error() == "agent did not reply in time" {
			writeError(w, http.StatusGatewayTimeout, "agent did not reply in time")
			return
		}
		slog.Error("probe: pub/sub failure", "error", err, "probe_id", probeID)
		writeError(w, http.StatusInternalServerError, "probe failed: "+err.Error())
		return
	}

	var result websocket.ProbeResultPayload
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		slog.Error("probe: invalid result payload", "error", err, "probe_id", probeID)
		writeError(w, http.StatusInternalServerError, "invalid agent reply")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     result.OK,
		"error":  result.Error,
		"detail": result.Detail,
	})
}

// decryptIfPossible returns the stored bytes as-is when no encryption key is
// configured (dev); otherwise AES-GCM decrypts.
func decryptIfPossible(stored []byte, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return stored, nil
	}
	return crypto.Decrypt(stored, key)
}
