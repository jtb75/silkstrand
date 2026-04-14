package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jtb75/silkstrand/api/internal/middleware"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/notify"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// NotificationChannelsHandler serves CRUD for outbound notification
// channels (ADR 003 D12). R1c-a ships webhook + slack; API rejects
// unknown / not-yet-supported types.
type NotificationChannelsHandler struct {
	store  store.Store
	encKey []byte
}

func NewNotificationChannelsHandler(s store.Store, encKey []byte) *NotificationChannelsHandler {
	return &NotificationChannelsHandler{store: s, encKey: encKey}
}

func (h *NotificationChannelsHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListNotificationChannels(r.Context())
	if err != nil {
		slog.Error("list channels", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if items == nil {
		items = []model.NotificationChannel{}
	}
	// Scrub secrets from responses — config returned in the UI never
	// carries the ciphertext blob back out.
	scrubbed := make([]model.NotificationChannel, len(items))
	for i, c := range items {
		scrubbed[i] = scrubChannelForWire(c)
	}
	writeJSON(w, http.StatusOK, scrubbed)
}

func (h *NotificationChannelsHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.GetNotificationChannel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	if c == nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, scrubChannelForWire(*c))
}

// POST /api/v1/notification-channels
// Body: { name, type, config, enabled? }
// config per type:
//
//	webhook : { url, secret? }       — secret plaintext in; stored encrypted
//	slack   : { webhook_url }        — url plaintext in; stored encrypted
func (h *NotificationChannelsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ch, err := h.parseChannel(r, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.store.CreateNotificationChannel(r.Context(), *ch)
	if err != nil {
		slog.Error("create channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create channel")
		return
	}
	writeJSON(w, http.StatusCreated, scrubChannelForWire(*out))
}

// PUT /api/v1/notification-channels/{id}
// Only fields sent are applied; secrets are re-encrypted.
func (h *NotificationChannelsHandler) Update(w http.ResponseWriter, r *http.Request) {
	existing, err := h.store.GetNotificationChannel(r.Context(), r.PathValue("id"))
	if err != nil || existing == nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	ch, err := h.parseChannel(r, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ch.ID = existing.ID
	out, err := h.store.UpdateNotificationChannel(r.Context(), *ch)
	if err != nil {
		slog.Error("update channel", "error", err)
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, scrubChannelForWire(*out))
}

func (h *NotificationChannelsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteNotificationChannel(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// parseChannel decodes a create/update request, validates the type,
// and encrypts any sensitive config fields.
func (h *NotificationChannelsHandler) parseChannel(r *http.Request, isNew bool) (*model.NotificationChannel, error) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		return nil, errors.New("unauthorized")
	}
	var req struct {
		Name    string          `json:"name"`
		Type    string          `json:"type"`
		Config  json.RawMessage `json:"config"`
		Enabled *bool           `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.New("invalid request body")
	}
	if isNew && req.Name == "" {
		return nil, errors.New("name is required")
	}
	switch req.Type {
	case model.ChannelTypeWebhook, model.ChannelTypeSlack:
	case model.ChannelTypeEmail, model.ChannelTypePagerDuty:
		return nil, errors.New("channel type " + req.Type + " not yet supported (R1.1)")
	default:
		return nil, errors.New("unknown channel type: " + req.Type)
	}
	encCfg, err := encryptSecretsInConfig(req.Type, req.Config, h.encKey)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return &model.NotificationChannel{
		TenantID: claims.TenantID,
		Name:     req.Name,
		Type:     req.Type,
		Config:   encCfg,
		Enabled:  enabled,
	}, nil
}

// encryptSecretsInConfig takes plaintext incoming config JSON and
// returns a marshalled config with sensitive fields replaced by
// encrypted ciphertext (ADR 004 C0 pattern).
func encryptSecretsInConfig(chType string, raw json.RawMessage, encKey []byte) (json.RawMessage, error) {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, errors.New("config must be a JSON object")
	}
	switch chType {
	case model.ChannelTypeWebhook:
		if url, ok := cfg["url"].(string); !ok || url == "" {
			return nil, errors.New("webhook config requires url")
		}
		if sec, ok := cfg["secret"].(string); ok && sec != "" {
			enc, err := notify.EncryptSecret(sec, encKey)
			if err != nil {
				return nil, err
			}
			cfg["secret"] = enc
		}
	case model.ChannelTypeSlack:
		sec, ok := cfg["webhook_url"].(string)
		if !ok || sec == "" {
			return nil, errors.New("slack config requires webhook_url")
		}
		enc, err := notify.EncryptSecret(sec, encKey)
		if err != nil {
			return nil, err
		}
		cfg["webhook_url"] = enc
	}
	return json.Marshal(cfg)
}

// scrubChannelForWire replaces sensitive config values with a sentinel
// so the UI can tell "a secret is set" without the secret itself ever
// round-tripping.
func scrubChannelForWire(c model.NotificationChannel) model.NotificationChannel {
	var cfg map[string]any
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return c
	}
	switch c.Type {
	case model.ChannelTypeWebhook:
		if _, ok := cfg["secret"]; ok {
			cfg["secret"] = "(set)"
		}
	case model.ChannelTypeSlack:
		if _, ok := cfg["webhook_url"]; ok {
			cfg["webhook_url"] = "(set)"
		}
	}
	if raw, err := json.Marshal(cfg); err == nil {
		c.Config = raw
	}
	return c
}
