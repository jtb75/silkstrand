// Package notify dispatches rule-engine alerts over pluggable outbound
// channels (ADR 003 D12). R1c-a ships webhook + Slack; email +
// PagerDuty are R1.1+.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// Severity values carried on a dispatch.
const (
	SeverityInfo     = "info"
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// Event is the payload the rule engine hands to the dispatcher when a
// `notify` action fires.
type Event struct {
	TenantID    string
	ChannelName string // rule action's "channel" field
	Severity    string
	Title       string
	Message     string
	AssetID     string
	RuleID      string
	RuleName    string
	EventID     string // asset_events id if triggered on an event
	Payload     map[string]any
}

// Dispatcher sends notifications asynchronously. Each send writes a
// notification_deliveries row (pending → sent | failed). Failed rows
// are reviewable in the UI and retriable by a future worker (R1c+).
type Dispatcher struct {
	store  store.Store
	encKey []byte
	client *http.Client
}

func New(s store.Store, encKey []byte) *Dispatcher {
	return &Dispatcher{
		store:  s,
		encKey: encKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// DispatchAsync queues a send in a fresh goroutine. Errors never
// propagate to the caller — they land in notification_deliveries.status
// for future retry.
func (d *Dispatcher) DispatchAsync(ev Event) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := d.dispatch(ctx, ev); err != nil {
			slog.Warn("notify.dispatch failed",
				"tenant", ev.TenantID, "channel", ev.ChannelName, "error", err)
		}
	}()
}

func (d *Dispatcher) dispatch(ctx context.Context, ev Event) error {
	channel, err := d.store.GetNotificationChannelByName(ctx, ev.TenantID, ev.ChannelName)
	if err != nil {
		return fmt.Errorf("looking up channel: %w", err)
	}
	if channel == nil {
		return d.recordFailure(ctx, ev, nil, 0, "channel not found: "+ev.ChannelName)
	}
	if !channel.Enabled {
		slog.Debug("notify.skip disabled channel", "channel", channel.Name)
		return nil
	}

	body, err := renderBody(ev, channel.Type)
	if err != nil {
		return d.recordFailure(ctx, ev, channel, 0, "render: "+err.Error())
	}
	switch channel.Type {
	case model.ChannelTypeWebhook:
		status, err := d.sendWebhook(ctx, channel, body)
		return d.record(ctx, ev, channel, status, body, err)
	case model.ChannelTypeSlack:
		status, err := d.sendSlack(ctx, channel, body)
		return d.record(ctx, ev, channel, status, body, err)
	default:
		return d.recordFailure(ctx, ev, channel, 0, "unsupported channel type: "+channel.Type)
	}
}

// --- webhook -----------------------------------------------------

type webhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"` // base64 AES-GCM ciphertext
}

func (d *Dispatcher) sendWebhook(ctx context.Context, channel *model.NotificationChannel, body []byte) (int, error) {
	cfg, err := decodeWebhookConfig(channel.Config, d.encKey)
	if err != nil {
		return 0, fmt.Errorf("webhook config: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SilkStrand/1")
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(body)
		req.Header.Set("X-SilkStrand-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("webhook HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func decodeWebhookConfig(raw json.RawMessage, encKey []byte) (*webhookConfig, error) {
	var cfg webhookConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.URL == "" {
		return nil, errors.New("webhook url missing")
	}
	if cfg.Secret != "" {
		plain, err := decryptSecret(cfg.Secret, encKey)
		if err != nil {
			return nil, fmt.Errorf("decrypting webhook secret: %w", err)
		}
		cfg.Secret = plain
	}
	return &cfg, nil
}

// --- slack -------------------------------------------------------

type slackConfig struct {
	WebhookURL string `json:"webhook_url"` // base64 AES-GCM ciphertext
}

func (d *Dispatcher) sendSlack(ctx context.Context, channel *model.NotificationChannel, body []byte) (int, error) {
	cfg, err := decodeSlackConfig(channel.Config, d.encKey)
	if err != nil {
		return 0, fmt.Errorf("slack config: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("slack HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func decodeSlackConfig(raw json.RawMessage, encKey []byte) (*slackConfig, error) {
	var cfg slackConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.WebhookURL == "" {
		return nil, errors.New("slack webhook_url missing")
	}
	plain, err := decryptSecret(cfg.WebhookURL, encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting slack webhook_url: %w", err)
	}
	cfg.WebhookURL = plain
	return &cfg, nil
}

// --- body rendering ---------------------------------------------

// renderBody produces a channel-appropriate JSON body. Webhooks get
// the raw Event envelope; Slack gets a blocks-lite message so the
// notification is human-readable in Slack without the user writing a
// template.
func renderBody(ev Event, channelType string) ([]byte, error) {
	switch channelType {
	case model.ChannelTypeSlack:
		text := ev.Title
		if ev.Message != "" {
			text = ev.Title + "\n" + ev.Message
		}
		return json.Marshal(map[string]any{
			"text": text,
			"attachments": []map[string]any{{
				"color": slackColor(ev.Severity),
				"fields": []map[string]any{
					{"title": "rule", "value": ev.RuleName, "short": true},
					{"title": "severity", "value": ev.Severity, "short": true},
				},
			}},
		})
	default:
		return json.Marshal(map[string]any{
			"schema":      "silkstrand-notify-v1",
			"tenant_id":   ev.TenantID,
			"severity":    ev.Severity,
			"title":       ev.Title,
			"message":     ev.Message,
			"asset_id":    ev.AssetID,
			"rule_id":     ev.RuleID,
			"rule_name":   ev.RuleName,
			"event_id":    ev.EventID,
			"payload":     ev.Payload,
			"dispatched_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func slackColor(sev string) string {
	switch sev {
	case SeverityCritical, SeverityHigh:
		return "#d32f2f"
	case SeverityMedium:
		return "#f57c00"
	case SeverityLow:
		return "#ffc107"
	default:
		return "#757575"
	}
}

// --- delivery records --------------------------------------------

func (d *Dispatcher) record(ctx context.Context, ev Event, channel *model.NotificationChannel, status int, body []byte, sendErr error) error {
	del := model.NotificationDelivery{
		TenantID:  ev.TenantID,
		ChannelID: channel.ID,
		Severity:  &ev.Severity,
		Status:    model.DeliveryStatusSent,
		Attempt:   1,
		Payload:   body,
	}
	if ev.RuleID != "" {
		del.RuleID = &ev.RuleID
	}
	if ev.EventID != "" {
		del.EventID = &ev.EventID
	}
	if status > 0 {
		del.ResponseCode = &status
	}
	if sendErr != nil {
		del.Status = model.DeliveryStatusFailed
		errStr := sendErr.Error()
		del.Error = &errStr
	}
	if err := d.store.InsertNotificationDelivery(ctx, del); err != nil {
		slog.Warn("notify.record failed", "error", err)
	}
	return sendErr
}

func (d *Dispatcher) recordFailure(ctx context.Context, ev Event, channel *model.NotificationChannel, status int, msg string) error {
	// notification_deliveries.channel_id is NOT NULL — no row when the
	// channel lookup itself failed. We still want an audit trail, so
	// log it at Warn.
	if channel == nil {
		slog.Warn("notify.channel_missing",
			"tenant", ev.TenantID, "channel", ev.ChannelName,
			"rule", ev.RuleName, "error", msg)
		return errors.New(msg)
	}
	del := model.NotificationDelivery{
		TenantID:  ev.TenantID,
		ChannelID: channel.ID,
		Severity:  &ev.Severity,
		Status:    model.DeliveryStatusFailed,
		Attempt:   1,
		Error:     &msg,
	}
	if ev.RuleID != "" {
		del.RuleID = &ev.RuleID
	}
	if status > 0 {
		del.ResponseCode = &status
	}
	if err := d.store.InsertNotificationDelivery(ctx, del); err != nil {
		slog.Warn("notify.record failure-record failed", "error", err)
	}
	return errors.New(msg)
}

// --- secret encryption helpers -----------------------------------

// EncryptSecret wraps a plaintext secret for storage in
// notification_channels.config. Returns base64(AES-256-GCM(secret)).
// Uses the same crypto plumbing as credential_sources.
func EncryptSecret(plain string, encKey []byte) (string, error) {
	if len(encKey) == 0 {
		// Dev only: pass-through, mirrors credential_sources behavior.
		return plain, nil
	}
	cipher, err := crypto.Encrypt([]byte(plain), encKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}

func decryptSecret(b64 string, encKey []byte) (string, error) {
	if len(encKey) == 0 {
		return b64, nil // dev mode
	}
	cipher, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	plain, err := crypto.Decrypt(cipher, encKey)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
