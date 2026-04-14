package websocket

import "encoding/json"

// Message type constants for the agent WebSocket protocol.
const (
	TypeDirective          = "directive"
	TypeScanStarted        = "scan_started"
	TypeScanResults        = "scan_results"
	TypeScanError          = "scan_error"
	TypeHeartbeat          = "heartbeat"
	TypeUpgrade            = "upgrade"
	TypeProbe              = "probe"
	TypeProbeResult        = "probe_result"
	TypeAssetDiscovered    = "asset_discovered"   // ADR 003 R1a
	TypeDiscoveryCompleted = "discovery_completed" // ADR 003 R1a
)

// UpgradePayload tells the agent to fetch a new binary and restart.
type UpgradePayload struct {
	Version          string            `json:"version"`
	BaseURL          string            `json:"base_url"`
	SHA256ByPlatform map[string]string `json:"sha256_by_platform"`
}

// ProbePayload validates target connectivity without running a scan.
type ProbePayload struct {
	ProbeID          string          `json:"probe_id"`
	TargetType       string          `json:"target_type"`
	TargetIdentifier string          `json:"target_identifier"`
	TargetConfig     json.RawMessage `json:"target_config"`
	Credentials      json.RawMessage `json:"credentials,omitempty"`
}

// ProbeResultPayload is the agent's reply to a probe.
type ProbeResultPayload struct {
	ProbeID string `json:"probe_id"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// DirectivePayload is sent from server to agent with scan instructions.
type DirectivePayload struct {
	ScanID           string          `json:"scan_id"`
	ScanType         string          `json:"scan_type,omitempty"` // "compliance" (default) | "discovery"
	BundleID         string          `json:"bundle_id"`
	BundleName       string          `json:"bundle_name"`
	BundleVersion    string          `json:"bundle_version"`
	BundleURL        string          `json:"bundle_url,omitempty"`
	TargetID         string          `json:"target_id"`
	TargetType       string          `json:"target_type"`
	TargetIdentifier string          `json:"target_identifier"`
	TargetConfig     json.RawMessage `json:"target_config"`
	Credentials      json.RawMessage `json:"credentials,omitempty"` // empty for discovery
}

// AssetDiscoveredPayload is sent from agent to server during a discovery
// scan, carrying a batch of discovered (ip, port, service, ...) tuples.
// The agent emits these incrementally per ADR 003 D9 — process inline,
// don't wait for discovery_completed.
type AssetDiscoveredPayload struct {
	ScanID   string                  `json:"scan_id"`
	BatchSeq int                     `json:"batch_seq,omitempty"`
	Stage    string                  `json:"stage,omitempty"` // naabu|httpx|nuclei
	Assets   []DiscoveredAssetUpsert `json:"assets"`
}

// DiscoveredAssetUpsert is one normalized asset finding the agent
// streams up. JSONB-shaped fields are pass-through.
type DiscoveredAssetUpsert struct {
	IP           string          `json:"ip"`
	Port         int             `json:"port"`
	Hostname     string          `json:"hostname,omitempty"`
	Service      string          `json:"service,omitempty"`
	Version      string          `json:"version,omitempty"`
	Technologies json.RawMessage `json:"technologies,omitempty"`
	CVEs         json.RawMessage `json:"cves,omitempty"`
	ObservedAt   string          `json:"observed_at"` // RFC3339; agent's clock
}

// DiscoveryCompletedPayload is sent from agent to server when a
// discovery scan finishes successfully.
type DiscoveryCompletedPayload struct {
	ScanID       string `json:"scan_id"`
	AssetsFound  int    `json:"assets_found"`
	HostsScanned int    `json:"hosts_scanned"`
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
func NewDirectiveMessage(scanID, scanType, bundleID, bundleName, bundleVersion, bundleURL, targetID, targetType, targetIdentifier string, targetConfig, credentials json.RawMessage) Message {
	payload := DirectivePayload{
		ScanID:           scanID,
		ScanType:         scanType,
		BundleID:         bundleID,
		BundleName:       bundleName,
		BundleVersion:    bundleVersion,
		BundleURL:        bundleURL,
		TargetID:         targetID,
		TargetType:       targetType,
		TargetIdentifier: targetIdentifier,
		TargetConfig:     targetConfig,
		Credentials:      credentials,
	}
	data, _ := json.Marshal(payload)
	return Message{Type: TypeDirective, Payload: data}
}
