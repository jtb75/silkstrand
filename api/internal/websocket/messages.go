package websocket

import "encoding/json"

// Message type constants for the agent WebSocket protocol.
const (
	TypeDirective    = "directive"
	TypeScanStarted  = "scan_started"
	TypeScanResults  = "scan_results"
	TypeScanError    = "scan_error"
	TypeHeartbeat    = "heartbeat"
	TypeUpgrade      = "upgrade"
)

// UpgradePayload tells the agent to fetch a new binary and restart.
type UpgradePayload struct {
	Version          string            `json:"version"`
	BaseURL          string            `json:"base_url"`
	SHA256ByPlatform map[string]string `json:"sha256_by_platform"`
}

// DirectivePayload is sent from server to agent with scan instructions.
type DirectivePayload struct {
	ScanID           string          `json:"scan_id"`
	BundleID         string          `json:"bundle_id"`
	BundleName       string          `json:"bundle_name"`
	BundleVersion    string          `json:"bundle_version"`
	TargetID         string          `json:"target_id"`
	TargetType       string          `json:"target_type"`
	TargetIdentifier string          `json:"target_identifier"`
	TargetConfig     json.RawMessage `json:"target_config"`
	Credentials      json.RawMessage `json:"credentials,omitempty"`
}

// ScanStartedPayload is sent from agent to server when scan execution begins.
type ScanStartedPayload struct {
	ScanID string `json:"scan_id"`
}

// ScanErrorPayload is sent from agent to server when scan execution fails.
type ScanErrorPayload struct {
	ScanID string `json:"scan_id"`
	Error  string `json:"error"`
}

// HeartbeatPayload is sent periodically from agent to server.
type HeartbeatPayload struct {
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// NewDirectiveMessage creates a Message containing an enriched scan directive.
func NewDirectiveMessage(scanID, bundleID, bundleName, bundleVersion, targetID, targetType, targetIdentifier string, targetConfig, credentials json.RawMessage) Message {
	payload := DirectivePayload{
		ScanID:           scanID,
		BundleID:         bundleID,
		BundleName:       bundleName,
		BundleVersion:    bundleVersion,
		TargetID:         targetID,
		TargetType:       targetType,
		TargetIdentifier: targetIdentifier,
		TargetConfig:     targetConfig,
		Credentials:      credentials,
	}
	data, _ := json.Marshal(payload)
	return Message{Type: TypeDirective, Payload: data}
}
