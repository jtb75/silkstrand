package handler

import (
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/websocket"
)

type AgentHandler struct {
	hub *websocket.Hub
}

func NewAgentHandler(hub *websocket.Hub) *AgentHandler {
	return &AgentHandler{hub: hub}
}

// Connect handles the WebSocket upgrade for agent connections.
func (h *AgentHandler) Connect(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id query parameter required")
		return
	}

	slog.Info("agent connecting", "agent_id", agentID)

	if err := h.hub.HandleConnection(w, r, agentID); err != nil {
		slog.Error("agent connection failed", "agent_id", agentID, "error", err)
	}
}
