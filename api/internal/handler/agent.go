package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

type AgentHandler struct {
	hub     *websocket.Hub
	store   store.Store
	ps      *pubsub.PubSub
	credKey []byte
}

func NewAgentHandler(hub *websocket.Hub, s store.Store, ps *pubsub.PubSub, credKey []byte) *AgentHandler {
	return &AgentHandler{hub: hub, store: s, ps: ps, credKey: credKey}
}

// Connect handles the WebSocket upgrade for agent connections.
// Validates the agent key and tenant status before upgrading to WebSocket.
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

	// Check tenant status — reject agents for suspended tenants
	tenant, err := h.store.GetTenantByID(r.Context(), agent.TenantID)
	if err != nil {
		slog.Error("looking up tenant for agent", "tenant_id", agent.TenantID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tenant == nil || tenant.Status != model.TenantStatusActive {
		slog.Warn("agent rejected: tenant not active", "agent_id", agentID, "tenant_id", agent.TenantID)
		writeError(w, http.StatusForbidden, "tenant suspended")
		return
	}

	// Start Redis subscription for this agent's directives
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	if h.ps != nil {
		go h.subscribeDirectives(ctx, agentID)
		go h.subscribeProbes(ctx, agentID)
	}

	// Update agent status to connected
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusConnected); err != nil {
		slog.Error("updating agent status", "agent_id", agentID, "error", err)
	}

	// HandleConnection blocks until the agent disconnects
	if err := h.hub.HandleConnection(w, r, agentID); err != nil {
		slog.Error("agent connection failed", "agent_id", agentID, "error", err)
	}

	// Agent disconnected — update status and fail any in-progress scans
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusDisconnected); err != nil {
		slog.Error("updating agent status on disconnect", "agent_id", agentID, "error", err)
	}

	if count, err := h.store.FailRunningScansForAgent(context.Background(), agentID); err != nil {
		slog.Error("failing scans on agent disconnect", "agent_id", agentID, "error", err)
	} else if count > 0 {
		slog.Warn("failed scans due to agent disconnect", "agent_id", agentID, "count", count)
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

// subscribeProbes forwards probe requests originating on any API instance
// to the agent's WebSocket. The probe handler publishes via Redis; only
// the instance that owns this WSS connection delivers it.
func (h *AgentHandler) subscribeProbes(ctx context.Context, agentID string) {
	err := h.ps.SubscribeProbes(ctx, agentID, func(payload []byte) {
		msg := websocket.Message{Type: websocket.TypeProbe, Payload: payload}
		if err := h.hub.Send(agentID, msg); err != nil {
			slog.Warn("forwarding probe to agent", "agent_id", agentID, "error", err)
		}
	})
	if err != nil && ctx.Err() == nil {
		slog.Error("probe subscription error", "agent_id", agentID, "error", err)
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

	// Look up and decrypt credentials (may be nil)
	var creds json.RawMessage
	encryptedCreds, _ := h.store.GetCredentialsByTarget(ctx, d.TargetID)
	if encryptedCreds != nil && len(h.credKey) > 0 {
		decrypted, err := crypto.Decrypt(encryptedCreds, h.credKey)
		if err != nil {
			slog.Error("decrypting credentials for directive", "target_id", d.TargetID, "error", err)
		} else {
			creds = json.RawMessage(decrypted)
		}
	} else if encryptedCreds != nil {
		// No encryption key configured — pass through as-is (local dev)
		creds = encryptedCreds
	}

	// Build enriched directive message
	var bundleURL string
	if bundle.GCSPath != nil {
		bundleURL = *bundle.GCSPath
	}
	msg := websocket.NewDirectiveMessage(
		d.ScanID, d.BundleID, bundle.Name, bundle.Version, bundleURL,
		d.TargetID, target.Type, target.Identifier, target.Config, creds,
	)

	if err := h.hub.Send(agentID, msg); err != nil {
		slog.Error("forwarding directive to agent", "agent_id", agentID, "scan_id", d.ScanID, "error", err)
	} else {
		slog.Info("forwarded directive to agent", "agent_id", agentID, "scan_id", d.ScanID)
	}
}

// SelfDelete is called by the agent itself during uninstall, authed with
// its own API key (the same key/hash used for the WSS connection). Deletes
// the agents row so the tenant admin UI no longer shows a ghost.
//
// Path: DELETE /api/v1/agents/self?agent_id={id}
// Auth: Bearer {agent_api_key}
func (h *AgentHandler) SelfDelete(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id query parameter required")
		return
	}
	key := extractBearerToken(r)
	if key == "" {
		writeError(w, http.StatusUnauthorized, "authorization required")
		return
	}
	agent, err := h.store.GetAgentByID(r.Context(), agentID)
	if err != nil {
		slog.Error("looking up agent for self-delete", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	if !verifyAgentKey(key, agent) {
		writeError(w, http.StatusUnauthorized, "invalid agent key")
		return
	}
	// store.DeleteAgent is tenant-scoped via context; set the tenant from the
	// agent row so the DELETE query's WHERE tenant_id matches.
	ctx := store.WithTenantID(r.Context(), agent.TenantID)
	if err := h.store.DeleteAgent(ctx, agentID); err != nil {
		slog.Error("self-deleting agent", "agent_id", agentID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	slog.Info("agent self-deleted", "agent_id", agentID, "tenant_id", agent.TenantID)
	w.WriteHeader(http.StatusNoContent)
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

	if agent.KeyHash != "" && constantTimeEqual(keyHash, agent.KeyHash) {
		return true
	}

	if agent.NextKeyHash != nil && *agent.NextKeyHash != "" && constantTimeEqual(keyHash, *agent.NextKeyHash) {
		return true
	}

	return false
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
