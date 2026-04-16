// Package logstream ships the agent's info+ slog output over the existing
// WSS tunnel so the control plane can surface live logs in the UI per
// ADR 008. Debug stays local; the stdout JSON handler is untouched.
//
// The package exposes three pieces:
//
//   - TunnelHandler: a slog.Handler that publishes each Enabled record as
//     a {type:"agent_log"} tunnel message, enriched with payload.scan_id
//     whenever the emitting goroutine's ctx carries one via WithScanID.
//   - MultiHandler: a tiny multiplexer (stdlib slog doesn't ship one) so
//     the agent's root logger can fan out to stdout + tunnel.
//   - WithScanID / ScanID: context plumbing for per-scan log filtering
//     (ADR 008 D4).
package logstream

import "context"

// scanIDKey is the ctx key under which per-scan log scope is stashed.
// Unexported to force callers through WithScanID/ScanID.
type scanIDKey struct{}

// WithScanID returns a derived context carrying scanID. When the
// TunnelHandler sees this ctx at emit time it stamps payload.scan_id so
// the SSE consumer can filter to a single scan's console (ADR 008 D4).
// An empty scanID returns ctx unchanged.
func WithScanID(ctx context.Context, scanID string) context.Context {
	if scanID == "" {
		return ctx
	}
	return context.WithValue(ctx, scanIDKey{}, scanID)
}

// ScanID reads the scan id stashed on ctx by WithScanID. Returns the
// empty string when absent; callers treat that as "not in a scan scope".
func ScanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(scanIDKey{}).(string)
	return v
}
