package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

type AgentHandler struct {
	hub   *websocket.Hub
	store store.Store
	ps    *pubsub.PubSub
}

func NewAgentHandler(hub *websocket.Hub, s store.Store, ps *pubsub.PubSub) *AgentHandler {
	return &AgentHandler{hub: hub, store: s, ps: ps}
}

// Connect handles the WebSocket upgrade for agent connections.
// Validates the agent key before upgrading to WebSocket.
func (h *AgentHandler) Connect(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id query parameter required")
		return
	}

	// Extract key from Authorization header
	key := extractBearerToken(r)
	if key == "" {
		writeError(w, http.StatusUnauthorized, "authorization required")
		return
	}

	// Look up agent (not tenant-scoped)
	agent, err := h.store.GetAgentByID(r.Context(), agentID)
	if err != nil {
		slog.Error("looking up agent", "agent_id", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Verify key against key_hash or next_key_hash
	if !verifyAgentKey(key, agent) {
		slog.Warn("agent key verification failed", "agent_id", agentID)
		writeError(w, http.StatusUnauthorized, "invalid agent key")
		return
	}

	slog.Info("agent authenticated", "agent_id", agentID, "tenant_id", agent.TenantID)

	// Start Redis subscription for this agent's directives
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if h.ps != nil {
		go h.subscribeDirectives(ctx, agentID)
	}

	// Update agent status to connected
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusConnected); err != nil {
		slog.Error("updating agent status", "agent_id", agentID, "error", err)
	}

	// HandleConnection blocks until the agent disconnects
	if err := h.hub.HandleConnection(w, r, agentID); err != nil {
		slog.Error("agent connection failed", "agent_id", agentID, "error", err)
	}

	// Agent disconnected — update status
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusDisconnected); err != nil {
		slog.Error("updating agent status on disconnect", "agent_id", agentID, "error", err)
	}

	slog.Info("agent disconnected", "agent_id", agentID)
}

func (h *AgentHandler) subscribeDirectives(ctx context.Context, agentID string) {
	err := h.ps.SubscribeDirectives(ctx, agentID, func(d pubsub.Directive) {
		h.forwardDirective(ctx, agentID, d)
	})
	if err != nil && ctx.Err() == nil {
		slog.Error("directive subscription error", "agent_id", agentID, "error", err)
	}
}

func (h *AgentHandler) forwardDirective(ctx context.Context, agentID string, d pubsub.Directive) {
	// Look up target to enrich the directive with connection details
	target, err := h.store.GetTargetByID(ctx, d.TargetID)
	if err != nil {
		slog.Error("looking up target for directive", "target_id", d.TargetID, "error", err)
		return
	}
	if target == nil {
		slog.Error("target not found for directive", "target_id", d.TargetID)
		return
	}

	// Look up bundle metadata
	bundle, err := h.store.GetBundle(ctx, d.BundleID)
	if err != nil {
		slog.Error("looking up bundle for directive", "bundle_id", d.BundleID, "error", err)
		return
	}
	if bundle == nil {
		slog.Error("bundle not found for directive", "bundle_id", d.BundleID)
		return
	}

	// Look up credentials (may be nil)
	creds, _ := h.store.GetCredentialsByTarget(ctx, d.TargetID)

	// Build enriched directive message
	msg := websocket.NewDirectiveMessage(
		d.ScanID, d.BundleID, bundle.Name, bundle.Version,
		d.TargetID, target.Type, target.Identifier, target.Config, creds,
	)

	if err := h.hub.Send(agentID, msg); err != nil {
		slog.Error("forwarding directive to agent", "agent_id", agentID, "scan_id", d.ScanID, "error", err)
	} else {
		slog.Info("forwarded directive to agent", "agent_id", agentID, "scan_id", d.ScanID)
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

func verifyAgentKey(key string, agent *model.Agent) bool {
	h := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(h[:])

	// Check primary key
	if agent.KeyHash != "" && constantTimeEqual(keyHash, agent.KeyHash) {
		return true
	}

	// Check rotation key
	if agent.NextKeyHash != nil && *agent.NextKeyHash != "" && constantTimeEqual(keyHash, *agent.NextKeyHash) {
		return true
	}

	return false
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
