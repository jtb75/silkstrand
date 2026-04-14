package tunnel

import "encoding/json"

// Message is the envelope for all WebSocket messages between agent and server.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Message type constants matching the API's websocket protocol.
const (
	TypeDirective          = "directive"
	TypeScanStarted        = "scan_started"
	TypeScanResults        = "scan_results"
	TypeScanError          = "scan_error"
	TypeHeartbeat          = "heartbeat"
	TypeUpgrade            = "upgrade"
	TypeProbe              = "probe"
	TypeProbeResult        = "probe_result"
	TypeAssetDiscovered    = "asset_discovered"    // ADR 003 R1a
	TypeDiscoveryCompleted = "discovery_completed" // ADR 003 R1a
	TypeAllowlistSnapshot  = "allowlist_snapshot"  // ADR 003 D11 — agent → server informational
)

// AllowlistSnapshotPayload is the agent's most recently reported
// scan allowlist (D11). Informational only — the agent remains the
// policy authority. Server uses it to tag discovered_assets with a
// display status so the UI can surface/gate promote.
type AllowlistSnapshotPayload struct {
	Hash         string   `json:"hash"`
	Allow        []string `json:"allow"`
	Deny         []string `json:"deny,omitempty"`
	RateLimitPPS int      `json:"rate_limit_pps,omitempty"`
}

// ProbePayload is sent from server to agent to validate target connectivity
// without running a scan.
type ProbePayload struct {
	ProbeID          string          `json:"probe_id"`
	TargetType       string          `json:"target_type"`
	TargetIdentifier string          `json:"target_identifier"`
	TargetConfig     json.RawMessage `json:"target_config"`
	Credentials      json.RawMessage `json:"credentials,omitempty"`
}

// ProbeResultPayload is the agent's reply.
type ProbeResultPayload struct {
	ProbeID string `json:"probe_id"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Detail  string `json:"detail,omitempty"` // e.g. "PostgreSQL 16.13"
}

// UpgradePayload instructs the agent to download a new binary version,
// swap in place, and exit so the service manager restarts it.
type UpgradePayload struct {
	Version          string            `json:"version"`  // e.g. "v0.1.4" or "latest"
	BaseURL          string            `json:"base_url"` // e.g. https://storage.googleapis.com/silkstrand-agent-releases
	SHA256ByPlatform map[string]string `json:"sha256_by_platform"`
	// Keys are "<os>-<arch>": linux-amd64, darwin-arm64, etc.
}

// DirectivePayload is received from the server with scan instructions.
type DirectivePayload struct {
	ScanID           string          `json:"scan_id"`
	ScanType         string          `json:"scan_type,omitempty"` // "compliance" (default) | "discovery"
	BundleID         string          `json:"bundle_id"`
	BundleName       string          `json:"bundle_name"`
	BundleVersion    string          `json:"bundle_version"`
	BundleURL        string          `json:"bundle_url,omitempty"` // HTTPS URL to a .tar.gz; agent fetches if not cached
	TargetID         string          `json:"target_id"`
	TargetType       string          `json:"target_type"`
	TargetIdentifier string          `json:"target_identifier"`
	TargetConfig     json.RawMessage `json:"target_config"`
	Credentials      json.RawMessage `json:"credentials,omitempty"`
}

// AssetDiscoveredPayload is sent during a discovery scan with a batch of
// findings. Process inline server-side per ADR 003 D9.
type AssetDiscoveredPayload struct {
	ScanID   string                  `json:"scan_id"`
	BatchSeq int                     `json:"batch_seq,omitempty"`
	Stage    string                  `json:"stage,omitempty"` // naabu|httpx|nuclei
	Assets   []DiscoveredAssetUpsert `json:"assets"`
}

// DiscoveredAssetUpsert is one normalized asset finding.
type DiscoveredAssetUpsert struct {
	IP           string          `json:"ip"`
	Port         int             `json:"port"`
	Hostname     string          `json:"hostname,omitempty"`
	Service      string          `json:"service,omitempty"`
	Version      string          `json:"version,omitempty"`
	Technologies json.RawMessage `json:"technologies,omitempty"`
	CVEs         json.RawMessage `json:"cves,omitempty"`
	ObservedAt   string          `json:"observed_at"`
}

// DiscoveryCompletedPayload is the terminal message for a discovery scan.
type DiscoveryCompletedPayload struct {
	ScanID       string `json:"scan_id"`
	AssetsFound  int    `json:"assets_found"`
	HostsScanned int    `json:"hosts_scanned"`
}

// ScanStartedPayload is sent to the server when scan execution begins.
type ScanStartedPayload struct {
	ScanID string `json:"scan_id"`
}

// ScanResultsPayload is sent to the server with completed scan results.
type ScanResultsPayload struct {
	ScanID  string          `json:"scan_id"`
	Results json.RawMessage `json:"results"`
}

// ScanErrorPayload is sent to the server when scan execution fails.
type ScanErrorPayload struct {
	ScanID string `json:"scan_id"`
	Error  string `json:"error"`
}

// HeartbeatPayload is sent periodically to the server.
type HeartbeatPayload struct {
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}
