package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
	"github.com/jtb75/silkstrand/api/internal/websocket"
)

const installTokenTTL = time.Hour

// AgentsHandler serves the tenant-facing agent CRUD API. Agents registered
// here get a one-time API key shown in the response; the hash is stored.
// See api/internal/handler/agent.go for the WebSocket connect handler.
type AgentsHandler struct {
	store       store.Store
	hub         *websocket.Hub
	ps          *pubsub.PubSub
	releasesURL string // base URL for agent binaries/installer, e.g. GCS bucket
}

func NewAgentsHandler(s store.Store, hub *websocket.Hub, ps *pubsub.PubSub, releasesURL string) *AgentsHandler {
	return &AgentsHandler{store: s, hub: hub, ps: ps, releasesURL: releasesURL}
}

// GET /api/v1/agents
func (h *AgentsHandler) List(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.ListAgents(r.Context())
	if err != nil {
		slog.Error("listing agents", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	if agents == nil {
		agents = []model.Agent{}
	}
	writeJSON(w, http.StatusOK, agents)
}

// GET /api/v1/agents/{id}
func (h *AgentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		slog.Error("getting agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

// GET /api/v1/agents/{id}/allowlist
// Returns the customer-owned scan allowlist snapshot the agent most
// recently reported over WSS (ADR 003 D11). The server has zero
// authority to edit it — this endpoint is purely a viewer so admins
// can see what the agent is willing to scan. 404 when the agent has
// never reported a snapshot (new agent, or running an older binary).
func (h *AgentsHandler) GetAllowlist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Tenant-scoped GetAgent enforces that the caller owns this agent.
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil {
		slog.Error("getting agent for allowlist", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	snap, err := h.store.GetAgentAllowlist(r.Context(), id)
	if err != nil {
		slog.Error("loading agent allowlist", "agent_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load allowlist")
		return
	}
	if snap == nil {
		writeError(w, http.StatusNotFound, "agent has not reported an allowlist yet")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// POST /api/v1/agents
// Body: {name, version?}
// Response includes the plaintext api_key — shown ONCE; the hash is stored.
func (h *AgentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Name    string `json:"name"`
		Version string `json:"version,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	agent, rawKey, err := h.store.CreateAgent(r.Context(), model.CreateAgentRequest{
		TenantID: claims.TenantID,
		Name:     req.Name,
		Version:  req.Version,
	})
	if err != nil {
		slog.Error("creating agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"agent":   agent,
		"api_key": rawKey, // shown once; store securely
	})
}

// POST /api/v1/agents/{id}/rotate-key
// Response includes new plaintext key (old one stays valid until the agent
// reconnects; that's how dual-key rotation works).
func (h *AgentsHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Verify tenant ownership before rotating.
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil || agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	rawKey, err := h.store.RotateAgentKey(r.Context(), id)
	if err != nil {
		slog.Error("rotating agent key", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to rotate key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": rawKey})
}

// DownloadInfo describes where agent binaries and the installer script live.
// The public GCS base URL is configured at the API level (AGENT_RELEASES_URL)
// and surfaced here so the tenant frontend doesn't need to hardcode it.
type DownloadInfo struct {
	Version       string            `json:"version"`
	InstallScript string            `json:"install_script"`
	InstallCmd    string            `json:"install_cmd"`
	Binaries      map[string]string `json:"binaries"`
}

// GET /api/v1/agents/downloads
func (h *AgentsHandler) Downloads(w http.ResponseWriter, r *http.Request) {
	base := h.releasesURL
	if base == "" {
		base = "https://storage.googleapis.com/silkstrand-agent-releases"
	}
	info := DownloadInfo{
		Version:       "latest",
		InstallScript: base + "/install.sh",
		InstallCmd:    "curl -sSL " + base + "/install.sh | sh",
		Binaries: map[string]string{
			"linux-amd64":       base + "/latest/silkstrand-agent-linux-amd64",
			"linux-arm64":       base + "/latest/silkstrand-agent-linux-arm64",
			"darwin-amd64":      base + "/latest/silkstrand-agent-darwin-amd64",
			"darwin-arm64":      base + "/latest/silkstrand-agent-darwin-arm64",
			"windows-amd64.exe": base + "/latest/silkstrand-agent-windows-amd64.exe",
		},
	}
	writeJSON(w, http.StatusOK, info)
}

// DELETE /api/v1/agents/{id}
func (h *AgentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.store.DeleteAgent(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "agent not found")
			return
		}
		slog.Error("deleting agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/agents/install-tokens (authenticated, tenant-scoped)
// Body: {} (no fields yet)
// Returns a one-time install token (1h, single-use) bound to this tenant.
// Used by install.sh to call /api/v1/agents/bootstrap and self-register.
func (h *AgentsHandler) CreateInstallToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	plaintext, tokenHash, err := crypto.NewInstallToken()
	if err != nil {
		slog.Error("generating install token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	expiresAt := time.Now().Add(installTokenTTL)
	createdBy := ""
	if claims.Sub != "" {
		createdBy = claims.Sub
	} else if claims.UserID != "" {
		createdBy = claims.UserID
	}
	if err := h.store.CreateInstallToken(r.Context(), claims.TenantID, tokenHash, expiresAt, createdBy); err != nil {
		slog.Error("storing install token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"install_token": plaintext,
		"expires_at":    expiresAt.UTC().Format(time.RFC3339),
	})
}

// POST /api/v1/agents/bootstrap (public, rate-limited)
// Body: {install_token, name, version?}
// Consumes the token (single-use) and creates an agent for the token's
// tenant. Returns long-lived agent_id + api_key. Tenant is derived from
// the token on the server — never trusted from the client.
func (h *AgentsHandler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstallToken string `json:"install_token"`
		Name         string `json:"name"`
		Version      string `json:"version,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InstallToken == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "install_token and name are required")
		return
	}

	// Create the agent BEFORE consuming the token so we have the agent_id to
	// audit on the token row. Small race window (agent exists briefly with
	// no token link) — acceptable; agents table isn't user-visible yet.
	// Caveat: we need the tenant_id first to create the agent. So we look it
	// up with a non-consuming read, then consume atomically after creation.

	// Check token validity (read-only) first to give a proper error up-front.
	// We can piggy-back on the UPDATE…RETURNING by doing a two-step: create
	// agent after a valid peek, then consume for real.
	// Simpler: consume first, roll back the agent if consume failed. Since we
	// don't have tx plumbing here, go with peek-then-create-then-consume.

	hash := crypto.HashInstallToken(req.InstallToken)

	// Consume (and get tenant_id) — use a placeholder agent_id since the
	// audit field is optional. Then create the agent. We set the audit
	// field with a follow-up UPDATE once we have the real id. If the agent
	// creation fails, the token is already used — that's a UX regression
	// but not a security issue (admin just generates a new token).
	tenantID, err := h.store.ConsumeInstallToken(r.Context(), hash, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "install token invalid, expired, or already used")
			return
		}
		slog.Error("consuming install token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}

	agent, rawKey, err := h.store.CreateAgent(r.Context(), model.CreateAgentRequest{
		TenantID: tenantID,
		Name:     req.Name,
		Version:  req.Version,
	})
	if err != nil {
		slog.Error("bootstrap: creating agent", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create agent")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"agent_id": agent.ID,
		"api_key":  rawKey,
	})
}

// UpgradeRequest is the body of POST /api/v1/agents/{id}/upgrade.
// Version defaults to "latest" when empty. sha256_by_platform is optional;
// if omitted the agent will download without checksum verification.
type UpgradeRequest struct {
	Version          string            `json:"version"`
	SHA256ByPlatform map[string]string `json:"sha256_by_platform,omitempty"`
}

// POST /api/v1/agents/{id}/upgrade (tenant-authed)
// Body: {version?, sha256_by_platform?}
// Sends an upgrade directive to a connected agent.
func (h *AgentsHandler) Upgrade(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Tenant-scoped GetAgent ensures the agent belongs to the caller's tenant.
	agent, err := h.store.GetAgent(r.Context(), id)
	if err != nil || agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req UpgradeRequest
	// Body is optional — tolerate empty / missing.
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Version == "" {
		req.Version = "latest"
	}

	baseURL := h.releasesURL
	if baseURL == "" {
		baseURL = "https://storage.googleapis.com/silkstrand-agent-releases"
	}

	payload, _ := json.Marshal(websocket.UpgradePayload{
		Version:          req.Version,
		BaseURL:          baseURL,
		SHA256ByPlatform: req.SHA256ByPlatform,
	})
	if h.ps == nil {
		writeError(w, http.StatusServiceUnavailable, "upgrade not available (no pubsub)")
		return
	}
	// Route through Redis pub/sub — the API instance handling this HTTP
	// request rarely owns the agent's WSS connection. Whichever instance
	// has the connection picks up the published message and delivers it.
	if err := h.ps.PublishUpgrade(r.Context(), id, payload); err != nil {
		slog.Error("publishing upgrade directive", "agent_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to dispatch upgrade")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "requested",
		"version": req.Version,
	})
}
