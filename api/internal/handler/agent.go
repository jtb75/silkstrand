package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/events"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

var base64Std = base64.StdEncoding

type AgentHandler struct {
	hub     *websocket.Hub
	store   store.Store
	ps      *pubsub.PubSub
	credKey []byte
	bus     events.Bus
}

func NewAgentHandler(hub *websocket.Hub, s store.Store, ps *pubsub.PubSub, credKey []byte, bus events.Bus) *AgentHandler {
	return &AgentHandler{hub: hub, store: s, ps: ps, credKey: credKey, bus: bus}
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
		go h.subscribeUpgrades(ctx, agentID)
	}

	// Update agent status to connected
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusConnected); err != nil {
		slog.Error("updating agent status", "agent_id", agentID, "error", err)
	}
	h.publishAgentStatus(agent.TenantID, agentID, "connected")

	// HandleConnection blocks until the agent disconnects
	if err := h.hub.HandleConnection(w, r, agentID); err != nil {
		slog.Error("agent connection failed", "agent_id", agentID, "error", err)
	}

	// Agent disconnected — update status and fail any in-progress scans
	if err := h.store.UpdateAgentStatus(context.Background(), agentID, model.AgentStatusDisconnected); err != nil {
		slog.Error("updating agent status on disconnect", "agent_id", agentID, "error", err)
	}
	h.publishAgentStatus(agent.TenantID, agentID, "disconnected")

	if count, err := h.store.FailRunningScansForAgent(context.Background(), agentID); err != nil {
		slog.Error("failing scans on agent disconnect", "agent_id", agentID, "error", err)
	} else if count > 0 {
		slog.Warn("failed scans due to agent disconnect", "agent_id", agentID, "count", count)
	}

	slog.Info("agent disconnected", "agent_id", agentID)
}

// publishAgentStatus emits an agent_status event on the bus so SSE
// subscribers (e.g. the tenant UI) can react to agent connect/disconnect/upgrade.
func (h *AgentHandler) publishAgentStatus(tenantID, agentID, status string) {
	if h.bus == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"status": status})
	if err := h.bus.Publish(context.Background(), events.Event{
		Kind:         "agent_status",
		ResourceType: "agent",
		ResourceID:   agentID,
		TenantID:     tenantID,
		OccurredAt:   time.Now(),
		Payload:      payload,
	}); err != nil {
		slog.Error("publishing agent_status event", "agent_id", agentID, "status", status, "error", err)
	}
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

// subscribeUpgrades forwards upgrade directives originating on any API
// instance to the agent's WebSocket. Mirrors subscribeProbes — only the
// instance that owns this WSS connection actually delivers the message.
func (h *AgentHandler) subscribeUpgrades(ctx context.Context, agentID string) {
	err := h.ps.SubscribeUpgrades(ctx, agentID, func(payload []byte) {
		msg := websocket.Message{Type: websocket.TypeUpgrade, Payload: payload}
		if err := h.hub.Send(agentID, msg); err != nil {
			slog.Warn("forwarding upgrade to agent", "agent_id", agentID, "error", err)
		}
	})
	if err != nil && ctx.Err() == nil {
		slog.Error("upgrade subscription error", "agent_id", agentID, "error", err)
	}
}

func (h *AgentHandler) forwardDirective(ctx context.Context, agentID string, d pubsub.Directive) {
	slog.Info("forwardDirective received",
		"scan_id", d.ScanID, "target_id", d.TargetID,
		"asset_endpoint_id", d.AssetEndpointID, "bundle_id", d.BundleID,
		"tenant_id", d.TenantID)
	// Resolve connection details. Endpoint-scoped compliance scans
	// don't have a target — derive host:port from the endpoint + parent asset.
	var targetID, targetType, targetIdentifier string
	var targetConfig json.RawMessage

	if d.TargetID != "" {
		target, err := h.store.GetTargetByID(ctx, d.TargetID)
		if err != nil {
			slog.Error("looking up target for directive", "target_id", d.TargetID, "error", err)
			return
		}
		if target == nil {
			slog.Error("target not found for directive", "target_id", d.TargetID)
			return
		}
		targetID = target.ID
		targetType = target.Type
		targetIdentifier = target.Identifier
		targetConfig = target.Config
	} else if d.AssetEndpointID != "" {
		ep, asset, err := h.store.GetAssetEndpointByID(ctx, d.AssetEndpointID)
		if err != nil {
			slog.Error("looking up endpoint for directive", "endpoint_id", d.AssetEndpointID, "error", err)
			return
		}
		if ep == nil || asset == nil {
			slog.Error("endpoint or asset not found for directive", "endpoint_id", d.AssetEndpointID)
			return
		}
		ip := ""
		if asset.PrimaryIP != nil {
			ip = *asset.PrimaryIP
		}
		svc := ""
		if ep.Service != nil {
			svc = *ep.Service
		}
		if svc == "" {
			svc = "database"
		}
		targetType = svc
		targetIdentifier = ip + ":" + strconv.Itoa(ep.Port)
		targetConfig, _ = json.Marshal(map[string]any{"port": ep.Port, "host": ip})
	} else {
		slog.Error("directive has no target or endpoint", "scan_id", d.ScanID)
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

	scanType := d.ScanType
	if scanType == "" {
		scanType = "compliance"
	}
	var creds json.RawMessage
	if scanType == "compliance" {
		creds = h.fetchCredentialForDirective(ctx, d)
	}

	var bundleURL string
	if bundle.GCSPath != nil {
		bundleURL = *bundle.GCSPath
	}
	msg := websocket.NewDirectiveMessage(
		d.ScanID, scanType, d.BundleID, bundle.Name, bundle.Version, bundleURL,
		targetID, targetType, targetIdentifier, targetConfig, creds,
	)

	if err := h.hub.Send(agentID, msg); err != nil {
		slog.Error("forwarding directive to agent", "agent_id", agentID, "scan_id", d.ScanID, "error", err)
	} else {
		slog.Info("forwarded directive to agent", "agent_id", agentID, "scan_id", d.ScanID, "scan_type", scanType)
	}
}

func (h *AgentHandler) fetchCredentialForDirective(ctx context.Context, d pubsub.Directive) json.RawMessage {
	// 1. Try the target-level credential_source (direct binding).
	if d.TargetID != "" {
		encryptedCreds, _, credErr := h.store.GetStaticCredentialForTarget(ctx, d.TargetID)
		switch {
		case credErr != nil:
			slog.Info("credential.fetch",
				"source_type", "static", "target_id", d.TargetID, "scan_id", d.ScanID,
				"outcome", "error", "error", credErr)
		case encryptedCreds != nil:
			return h.decryptCredential(encryptedCreds, d.ScanID, d.TargetID, "target")
		default:
			slog.Info("credential.fetch",
				"source_type", "static", "target_id", d.TargetID, "scan_id", d.ScanID,
				"outcome", "miss")
		}
	}

	// 2. Fall back to credential_mappings: find a credential_source
	//    mapped to any collection containing this scan's endpoint.
	if d.AssetEndpointID != "" && d.TenantID != "" {
		cs, err := h.store.ResolveCredentialForEndpoint(ctx, d.TenantID, d.AssetEndpointID)
		if err != nil {
			slog.Warn("credential.fetch.mapping_resolve",
				"endpoint", d.AssetEndpointID, "scan_id", d.ScanID, "error", err)
		} else if cs != nil {
			return h.decryptStaticSource(cs, d.ScanID, d.AssetEndpointID)
		}
	}

	return nil
}

// decryptCredential decrypts a raw credential blob (from GetStaticCredentialForTarget).
func (h *AgentHandler) decryptCredential(encrypted []byte, scanID, ref, via string) json.RawMessage {
	if len(h.credKey) == 0 {
		slog.Info("credential.fetch", "via", via, "ref", ref, "scan_id", scanID, "outcome", "ok_plaintext")
		return encrypted
	}
	decrypted, err := crypto.Decrypt(encrypted, h.credKey)
	if err != nil {
		slog.Info("credential.fetch", "via", via, "ref", ref, "scan_id", scanID,
			"outcome", "decrypt_error", "error", err)
		return nil
	}
	slog.Info("credential.fetch", "via", via, "ref", ref, "scan_id", scanID, "outcome", "ok")
	return decrypted
}

// decryptStaticSource extracts and decrypts the credential from a
// credential_source row resolved via credential_mappings.
func (h *AgentHandler) decryptStaticSource(cs *model.CredentialSource, scanID, endpointID string) json.RawMessage {
	var cfg model.StaticCredentialConfig
	if err := json.Unmarshal(cs.Config, &cfg); err != nil {
		slog.Warn("credential.fetch.mapping_parse",
			"source_id", cs.ID, "scan_id", scanID, "error", err)
		return nil
	}
	data, err := base64Decode(cfg.EncryptedData)
	if err != nil {
		slog.Warn("credential.fetch.mapping_decode",
			"source_id", cs.ID, "scan_id", scanID, "error", err)
		return nil
	}
	return h.decryptCredential(data, scanID, endpointID, "mapping")
}

func base64Decode(s string) ([]byte, error) {
	return base64Std.DecodeString(s)
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
