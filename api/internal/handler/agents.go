package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// AgentsHandler serves the tenant-facing agent CRUD API. Agents registered
// here get a one-time API key shown in the response; the hash is stored.
// See api/internal/handler/agent.go for the WebSocket connect handler.
type AgentsHandler struct {
	store       store.Store
	releasesURL string // base URL for agent binaries/installer, e.g. GCS bucket
}

func NewAgentsHandler(s store.Store, releasesURL string) *AgentsHandler {
	return &AgentsHandler{store: s, releasesURL: releasesURL}
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
