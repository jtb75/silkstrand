// Package notify dispatches rule-engine alerts over pluggable outbound
// channels. Post-ADR 006 P6 channels are credential_sources rows
// (type=slack|webhook|email|pagerduty) rather than a separate table.
// Rule actions reference channels by credential_source_id.
//
// P2 ships webhook + Slack (HMAC-signed webhooks + Slack incoming-webhook
// URLs). Email and PagerDuty are stubbed: the dispatcher records a
// notification_deliveries row with status='pending' and returns without
// attempting a send. Tracked as a P6 follow-on — requires an SMTP/Resend
// sender (with D14 per-tenant templates) for email and the Events API v2
// flow for PagerDuty. A retry worker will later consume 'pending' rows.
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

	silkcrypto "github.com/jtb75/silkstrand/api/internal/crypto"
	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

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
	TenantID          string
	ChannelSourceID   string
	Severity          string
	Title             string
	Message           string
	AssetID           string
	AssetEndpointID   string
	RuleID            string
	RuleName          string
	EventID           string
	Payload           map[string]any
}

// Dispatcher sends notifications asynchronously. Each send writes a
// notification_deliveries row (sent | failed).
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
// propagate to the caller — they land in notification_deliveries.
func (d *Dispatcher) DispatchAsync(ev Event) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := d.dispatch(ctx, ev); err != nil {
			slog.Warn("notify.dispatch_failed",
				"tenant", ev.TenantID, "channel_source", ev.ChannelSourceID, "error", err)
		}
	}()
}

func (d *Dispatcher) dispatch(ctx context.Context, ev Event) error {
	if ev.ChannelSourceID == "" {
		return errors.New("notify: missing credential_source_id")
	}
	src, err := d.store.GetCredentialSource(ctx, ev.ChannelSourceID)
	if err != nil {
		return fmt.Errorf("looking up channel source: %w", err)
	}
	if src == nil {
		return d.recordFailure(ctx, ev, nil, 0, "channel credential_source not found")
	}
	body, err := renderBody(ev, src.Type)
	if err != nil {
		return d.recordFailure(ctx, ev, src, 0, "render: "+err.Error())
	}
	switch src.Type {
	case model.CredentialSourceTypeWebhook:
		status, sendErr := d.sendWebhook(ctx, src, body)
		return d.record(ctx, ev, src, status, body, sendErr)
	case model.CredentialSourceTypeSlack:
		status, sendErr := d.sendSlack(ctx, src, body)
		return d.record(ctx, ev, src, status, body, sendErr)
	case model.CredentialSourceTypeEmail, model.CredentialSourceTypePagerDuty:
		// TODO(P6 follow-on): email + PagerDuty senders are stubbed.
		// The dispatcher records a notification_deliveries row with
		// status='pending' and returns without attempting a send. A real
		// send path requires:
		//   * Email: SMTP / Resend integration (pluggable mailer, mirror
		//     backoffice/internal/mailer) — honor FROM_EMAIL + per-tenant
		//     template selection (D14).
		//   * PagerDuty: Events API v2 (routing_key in credential_source
		//     config, dedup_key derived from rule+asset_endpoint).
		// Webhook + Slack HMAC signing already landed — see sendWebhook.
		// Once retry worker exists, it will pick up these 'pending' rows
		// and drive them to sent/failed.
		return d.recordPending(ctx, ev, src, "channel type not yet implemented: "+src.Type)
	default:
		return d.recordFailure(ctx, ev, src, 0, "unsupported channel source type: "+src.Type)
	}
}

// --- webhook -----------------------------------------------------

type webhookConfig struct {
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"` // base64 AES-GCM ciphertext
}

func (d *Dispatcher) sendWebhook(ctx context.Context, src *model.CredentialSource, body []byte) (int, error) {
	cfg, err := decodeWebhookConfig(src.Config, d.encKey)
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

func (d *Dispatcher) sendSlack(ctx context.Context, src *model.CredentialSource, body []byte) (int, error) {
	cfg, err := decodeSlackConfig(src.Config, d.encKey)
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

func renderBody(ev Event, channelType string) ([]byte, error) {
	switch channelType {
	case model.CredentialSourceTypeSlack:
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
			"schema":            "silkstrand-notify-v1",
			"tenant_id":         ev.TenantID,
			"severity":          ev.Severity,
			"title":             ev.Title,
			"message":           ev.Message,
			"asset_id":          ev.AssetID,
			"asset_endpoint_id": ev.AssetEndpointID,
			"rule_id":           ev.RuleID,
			"rule_name":         ev.RuleName,
			"event_id":          ev.EventID,
			"payload":           ev.Payload,
			"dispatched_at":     time.Now().UTC().Format(time.RFC3339),
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

func (d *Dispatcher) record(ctx context.Context, ev Event, src *model.CredentialSource, status int, body []byte, sendErr error) error {
	del := model.NotificationDelivery{
		TenantID:        ev.TenantID,
		ChannelSourceID: src.ID,
		Severity:        &ev.Severity,
		Status:          model.DeliveryStatusSent,
		Attempt:         1,
		Payload:         body,
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
		slog.Warn("notify.record_failed", "error", err)
	}
	return sendErr
}

// recordPending writes a notification_deliveries row with status='pending'
// for channel types that are not yet implemented (email, pagerduty). The
// future retry worker will consume these rows and drive them to
// sent/failed. No send attempt is made.
func (d *Dispatcher) recordPending(ctx context.Context, ev Event, src *model.CredentialSource, note string) error {
	del := model.NotificationDelivery{
		TenantID:        ev.TenantID,
		ChannelSourceID: src.ID,
		Severity:        &ev.Severity,
		Status:          model.DeliveryStatusPending,
		Attempt:         0,
		Error:           &note,
	}
	if ev.RuleID != "" {
		del.RuleID = &ev.RuleID
	}
	if ev.EventID != "" {
		del.EventID = &ev.EventID
	}
	if err := d.store.InsertNotificationDelivery(ctx, del); err != nil {
		slog.Warn("notify.record_pending_failed", "error", err)
	}
	slog.Info("notify.pending_stub", "channel_type", src.Type, "tenant", ev.TenantID, "rule", ev.RuleName)
	return nil
}

func (d *Dispatcher) recordFailure(ctx context.Context, ev Event, src *model.CredentialSource, status int, msg string) error {
	if src == nil {
		slog.Warn("notify.channel_missing",
			"tenant", ev.TenantID, "channel_source", ev.ChannelSourceID,
			"rule", ev.RuleName, "error", msg)
		return errors.New(msg)
	}
	del := model.NotificationDelivery{
		TenantID:        ev.TenantID,
		ChannelSourceID: src.ID,
		Severity:        &ev.Severity,
		Status:          model.DeliveryStatusFailed,
		Attempt:         1,
		Error:           &msg,
	}
	if ev.RuleID != "" {
		del.RuleID = &ev.RuleID
	}
	if status > 0 {
		del.ResponseCode = &status
	}
	if err := d.store.InsertNotificationDelivery(ctx, del); err != nil {
		slog.Warn("notify.record_failure_failed", "error", err)
	}
	return errors.New(msg)
}

// --- secret encryption helpers -----------------------------------

func EncryptSecret(plain string, encKey []byte) (string, error) {
	if len(encKey) == 0 {
		return plain, nil
	}
	cipher, err := silkcrypto.Encrypt([]byte(plain), encKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}

func decryptSecret(b64 string, encKey []byte) (string, error) {
	if len(encKey) == 0 {
		return b64, nil
	}
	cipher, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	plain, err := silkcrypto.Decrypt(cipher, encKey)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
