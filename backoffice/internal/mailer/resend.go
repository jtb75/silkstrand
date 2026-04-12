package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Resend sends email via the Resend transactional email API
// (https://resend.com/docs/api-reference/emails/send-email).
type Resend struct {
	APIKey    string
	FromEmail string // e.g. "SilkStrand <noreply@silkstrand.io>"
	http      *http.Client
}

func NewResend(apiKey, fromEmail string) *Resend {
	return &Resend{
		APIKey:    apiKey,
		FromEmail: fromEmail,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
	Text    string   `json:"text"`
}

func (r *Resend) send(to, subject, text, html string) error {
	body, err := json.Marshal(resendRequest{
		From:    r.FromEmail,
		To:      []string{to},
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
	if err != nil {
		return fmt.Errorf("marshaling resend request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("resend send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return fmt.Errorf("resend returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (r *Resend) SendInvite(to, inviteURL, tenantName string) error {
	subject := fmt.Sprintf("You're invited to %s on SilkStrand", tenantName)
	text := fmt.Sprintf(
		"You've been invited to join %s on SilkStrand.\n\nAccept your invitation:\n%s\n\nThis link expires in 7 days.",
		tenantName, inviteURL)
	html := fmt.Sprintf(`<!doctype html>
<html><body style="font-family:sans-serif;max-width:560px;margin:40px auto;padding:0 20px;color:#111">
<h2 style="margin:0 0 16px">You're invited to %s</h2>
<p>You've been invited to join <strong>%s</strong> on SilkStrand.</p>
<p style="margin:24px 0">
  <a href="%s" style="display:inline-block;background:#0f766e;color:#fff;padding:10px 20px;border-radius:6px;text-decoration:none">Accept invitation</a>
</p>
<p style="font-size:13px;color:#555">Or paste this link into your browser:<br><a href="%s">%s</a></p>
<p style="font-size:13px;color:#555">This link expires in 7 days.</p>
</body></html>`, tenantName, tenantName, inviteURL, inviteURL, inviteURL)
	return r.send(to, subject, text, html)
}

func (r *Resend) SendPasswordReset(to, resetURL string) error {
	subject := "Reset your SilkStrand password"
	text := fmt.Sprintf(
		"We received a request to reset your SilkStrand password.\n\nReset it here:\n%s\n\nThis link expires in 1 hour. If you didn't request a reset, ignore this email.",
		resetURL)
	html := fmt.Sprintf(`<!doctype html>
<html><body style="font-family:sans-serif;max-width:560px;margin:40px auto;padding:0 20px;color:#111">
<h2 style="margin:0 0 16px">Reset your password</h2>
<p>We received a request to reset your SilkStrand password.</p>
<p style="margin:24px 0">
  <a href="%s" style="display:inline-block;background:#0f766e;color:#fff;padding:10px 20px;border-radius:6px;text-decoration:none">Reset password</a>
</p>
<p style="font-size:13px;color:#555">Or paste this link into your browser:<br><a href="%s">%s</a></p>
<p style="font-size:13px;color:#555">This link expires in 1 hour. If you didn't request a reset, ignore this email.</p>
</body></html>`, resetURL, resetURL, resetURL)
	return r.send(to, subject, text, html)
}
