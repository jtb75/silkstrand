// Package allowlistreport periodically reads the customer-owned scan
// allowlist and ships a snapshot to the server over the WSS tunnel
// (ADR 003 D11 follow-up). Informational only: the agent's recon runner
// remains the sole gate on what gets scanned. The server uses the
// snapshot purely to label discovered_assets in the UI.
package allowlistreport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/jtb75/silkstrand/agent/internal/tunnel"
	"gopkg.in/yaml.v3"
)

const (
	// pollEvery is slow on purpose — the allowlist changes at human
	// speed, and the server dedupes by hash on its side.
	pollEvery = 60 * time.Second
)

// Sender is the subset of *tunnel.Tunnel the reporter needs.
type Sender interface {
	Send(tunnel.Message)
}

type yamlShape struct {
	Allow        []string `yaml:"allow"`
	Deny         []string `yaml:"deny"`
	RateLimitPPS int      `yaml:"rate_limit_pps"`
}

// Run polls the allowlist at `path` and sends a snapshot on first load
// and whenever the file contents change. Blocks until ctx is cancelled.
// A missing or unreadable file is skipped silently — absence of a
// snapshot on the server means "unknown", which is the intended default.
func Run(ctx context.Context, tun Sender, path string) {
	var lastHash string
	send := func() {
		hash, payload, ok := read(path)
		if !ok {
			return
		}
		if hash == lastHash {
			return
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			slog.Warn("marshal allowlist snapshot", "error", err)
			return
		}
		tun.Send(tunnel.Message{Type: tunnel.TypeAllowlistSnapshot, Payload: raw})
		lastHash = hash
		slog.Info("allowlist snapshot sent", "hash", hash, "allow", len(payload.Allow), "deny", len(payload.Deny))
	}

	send()
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			send()
		}
	}
}

func read(path string) (string, tunnel.AllowlistSnapshotPayload, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("reading allowlist", "path", path, "error", err)
		}
		return "", tunnel.AllowlistSnapshotPayload{}, false
	}
	var y yamlShape
	if err := yaml.Unmarshal(raw, &y); err != nil {
		slog.Warn("parsing allowlist yaml", "path", path, "error", err)
		return "", tunnel.AllowlistSnapshotPayload{}, false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), tunnel.AllowlistSnapshotPayload{
		Hash:         hex.EncodeToString(sum[:]),
		Allow:        y.Allow,
		Deny:         y.Deny,
		RateLimitPPS: y.RateLimitPPS,
	}, true
}
