package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// CredentialsHandler serves the tenant-facing credential API for targets.
// Credentials are stored encrypted at rest (AES-256-GCM); the API never
// returns plaintext after creation — only a presence indicator + type.
type CredentialsHandler struct {
	store   store.Store
	encKey  []byte // optional in dev; required in prod via CREDENTIAL_ENCRYPTION_KEY
}

func NewCredentialsHandler(s store.Store, encKey []byte) *CredentialsHandler {
	return &CredentialsHandler{store: s, encKey: encKey}
}

// GET /api/v1/targets/{id}/credential
// Returns 200 {set: true, type: "database"} or 200 {set: false}.
// Never returns the ciphertext or plaintext.
func (h *CredentialsHandler) Get(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	// Verify the target belongs to this tenant before answering.
	target, err := h.store.GetTarget(r.Context(), targetID)
	if err != nil || target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	has, credType, err := h.store.HasCredentialForTarget(r.Context(), targetID)
	if err != nil {
		slog.Error("checking credential", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	resp := map[string]any{"set": has}
	if has {
		resp["type"] = credType
	}
	writeJSON(w, http.StatusOK, resp)
}

// PUT /api/v1/targets/{id}/credential
// Body: {type: "database", data: {<secret fields>}}
// Upserts the credential — replaces any existing one for this target.
func (h *CredentialsHandler) Put(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	targetID := r.PathValue("id")
	target, err := h.store.GetTarget(r.Context(), targetID)
	if err != nil || target == nil {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}

	var req struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "type and data are required")
		return
	}

	var stored []byte
	if len(h.encKey) > 0 {
		stored, err = crypto.Encrypt(req.Data, h.encKey)
		if err != nil {
			slog.Error("encrypting credential", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to encrypt credential")
			return
		}
	} else {
		// Dev only: store plaintext if no encryption key configured.
		stored = req.Data
	}

	if err := h.store.UpsertStaticCredentialSource(r.Context(), claims.TenantID, targetID, req.Type, stored); err != nil {
		slog.Error("upserting credential source", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save credential")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/targets/{id}/credential
func (h *CredentialsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	targetID := r.PathValue("id")
	if err := h.store.DeleteCredentialForTarget(r.Context(), claims.TenantID, targetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no credential set for this target")
			return
		}
		slog.Error("deleting credential", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete credential")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
