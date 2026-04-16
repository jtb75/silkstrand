package logstream

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jtb75/silkstrand/agent/internal/tunnel"
)

// mockTunnel captures Send calls for assertion.
type mockTunnel struct {
	mu   sync.Mutex
	msgs []tunnel.Message
}

func (m *mockTunnel) Send(msg tunnel.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
}

func (m *mockTunnel) messages() []tunnel.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]tunnel.Message, len(m.msgs))
	copy(out, m.msgs)
	return out
}

// decodePayload is a tiny helper to keep individual tests readable.
func decodePayload(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return p
}

func TestTunnelHandler_EnabledFiltersBelowInfo(t *testing.T) {
	h := NewTunnelHandler(&mockTunnel{})
	cases := []struct {
		level slog.Level
		want  bool
	}{
		{slog.LevelDebug, false},
		{slog.LevelInfo, true},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}
	for _, tc := range cases {
		if got := h.Enabled(context.Background(), tc.level); got != tc.want {
			t.Errorf("Enabled(%v) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

func TestTunnelHandler_PayloadShape(t *testing.T) {
	mt := &mockTunnel{}
	h := NewTunnelHandler(mt)
	logger := slog.New(h)

	logger.Info("hello world", "key1", "value1", "count", 42)

	msgs := mt.messages()
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Type != TypeAgentLog {
		t.Errorf("Type = %q, want %q", msgs[0].Type, TypeAgentLog)
	}
	p := decodePayload(t, msgs[0].Payload)
	if p["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", p["level"])
	}
	if p["msg"] != "hello world" {
		t.Errorf("msg = %v", p["msg"])
	}
	if _, ok := p["scan_id"]; ok {
		t.Errorf("scan_id unexpectedly present: %v", p["scan_id"])
	}
	attrs, ok := p["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs missing / wrong type: %#v", p["attrs"])
	}
	if attrs["key1"] != "value1" {
		t.Errorf("attrs.key1 = %v", attrs["key1"])
	}
	// JSON decodes numbers as float64 — that's fine for assertions.
	if attrs["count"] != float64(42) {
		t.Errorf("attrs.count = %v (%T)", attrs["count"], attrs["count"])
	}
}

func TestTunnelHandler_ScanIDFromContext(t *testing.T) {
	mt := &mockTunnel{}
	h := NewTunnelHandler(mt)
	logger := slog.New(h)

	ctx := WithScanID(context.Background(), "scan-abc")
	logger.InfoContext(ctx, "inside scan")

	msgs := mt.messages()
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d", len(msgs))
	}
	p := decodePayload(t, msgs[0].Payload)
	if p["scan_id"] != "scan-abc" {
		t.Errorf("scan_id = %v, want scan-abc", p["scan_id"])
	}
}

func TestTunnelHandler_DebugIsDropped(t *testing.T) {
	mt := &mockTunnel{}
	h := NewTunnelHandler(mt)
	// slog.Logger applies Enabled() itself, so Debug() won't even call
	// Handle. That's precisely the ADR 008 D2 contract. Assert both the
	// Enabled gate and the end-to-end outcome.
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatalf("Enabled(Debug) = true, want false")
	}
	logger := slog.New(h)
	logger.Debug("should not ship")
	if msgs := mt.messages(); len(msgs) != 0 {
		t.Errorf("len(msgs) = %d, want 0", len(msgs))
	}
}

func TestTunnelHandler_RateLimitAndThrottleSummary(t *testing.T) {
	mt := &mockTunnel{}
	// Fixed clock so token-bucket refill is deterministic and the
	// throttle summary window is controllable.
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}

	// Tiny bucket so a second record trips the limit immediately.
	h := NewTunnelHandler(mt,
		WithNow(clock.Now),
		WithRateLimit(1, 1),
	)
	logger := slog.New(h)

	// 1st record: allowed, consumes the only token. Then 2nd is
	// dropped — the first drop triggers the opening throttle summary
	// (lastSummaryEmit was zero). Subsequent drops within the 5s
	// window accumulate into the counter without extra summaries.
	logger.Info("one")
	logger.Info("two")   // drop #1 → emits opening summary with dropped=1
	logger.Info("three") // drop #2 → counter++, no new summary yet
	logger.Info("four")  // drop #3 → counter++, no new summary yet

	msgs := mt.messages()
	// Expect: info "one" + opening throttle summary (count=1). The
	// subsequent drops bump the counter; they don't emit.
	if len(msgs) != 2 {
		t.Fatalf("initial phase: len(msgs) = %d, want 2; got %+v", len(msgs), msgs)
	}
	first := decodePayload(t, msgs[1].Payload)
	if first["msg"] != "agent_log.throttled" {
		t.Fatalf("msgs[1].msg = %v, want agent_log.throttled", first["msg"])
	}
	firstAttrs, _ := first["attrs"].(map[string]any)
	if firstAttrs["dropped"] != float64(1) {
		t.Errorf("opening summary dropped = %v, want 1", firstAttrs["dropped"])
	}

	// Advance past the summary window and trigger the throttle check
	// again. Outstanding counter = 2 (three + four).
	clock.advance(throttleSummaryWindow + time.Second)
	h.maybeEmitThrottled()

	msgs = mt.messages()
	if len(msgs) != 3 {
		t.Fatalf("after window advance: len(msgs) = %d, want 3; got %+v", len(msgs), msgs)
	}
	second := decodePayload(t, msgs[2].Payload)
	if second["msg"] != "agent_log.throttled" {
		t.Errorf("second summary msg = %v, want agent_log.throttled", second["msg"])
	}
	secondAttrs, _ := second["attrs"].(map[string]any)
	if secondAttrs["dropped"] != float64(2) {
		t.Errorf("second summary dropped = %v, want 2", secondAttrs["dropped"])
	}
}

func TestTunnelHandler_WithAttrsAndGroupMerge(t *testing.T) {
	mt := &mockTunnel{}
	h := NewTunnelHandler(mt)
	logger := slog.New(h).With("component", "recon").WithGroup("scan")

	logger.Info("stage", "id", "s1", "stage", "naabu")

	msgs := mt.messages()
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d", len(msgs))
	}
	p := decodePayload(t, msgs[0].Payload)
	attrs, _ := p["attrs"].(map[string]any)
	if attrs == nil {
		t.Fatalf("attrs missing")
	}
	// 'component' was attached before WithGroup → no group prefix.
	if attrs["component"] != "recon" {
		t.Errorf("attrs.component = %v, want recon", attrs["component"])
	}
	// Record attrs after WithGroup → prefixed.
	if attrs["scan.id"] != "s1" {
		t.Errorf("attrs[scan.id] = %v, want s1", attrs["scan.id"])
	}
	if attrs["scan.stage"] != "naabu" {
		t.Errorf("attrs[scan.stage] = %v, want naabu", attrs["scan.stage"])
	}
}

func TestMultiHandler_FansOutAndFiltersPerHandler(t *testing.T) {
	var buf bytes.Buffer
	// stdout handler accepts debug; tunnel handler only info+.
	stdout := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	mt := &mockTunnel{}
	tunnelH := NewTunnelHandler(mt)

	multi := NewMulti(stdout, tunnelH)
	logger := slog.New(multi)

	logger.Debug("debug-only", "k", "v")
	logger.Info("info-both", "k", "v")

	// Stdout should see both.
	stdoutLines := bytes.Count(bytes.TrimSpace(buf.Bytes()), []byte("\n")) + 1
	if stdoutLines != 2 {
		t.Errorf("stdout lines = %d, want 2 (buf=%q)", stdoutLines, buf.String())
	}

	// Tunnel should see only the Info one.
	msgs := mt.messages()
	if len(msgs) != 1 {
		t.Fatalf("tunnel msgs = %d, want 1", len(msgs))
	}
	p := decodePayload(t, msgs[0].Payload)
	if p["msg"] != "info-both" {
		t.Errorf("tunnel msg = %v", p["msg"])
	}
}

func TestMultiHandler_EnabledReportsAny(t *testing.T) {
	noop := slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError})
	tunnelH := NewTunnelHandler(&mockTunnel{})
	multi := NewMulti(noop, tunnelH)
	// noop says no for Info, tunnelH says yes — any-match wins.
	if !multi.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("MultiHandler.Enabled(Info) = false, want true")
	}
	if multi.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("MultiHandler.Enabled(Debug) = true, want false")
	}
}

func TestScanIDContextHelpers(t *testing.T) {
	if got := ScanID(context.Background()); got != "" {
		t.Errorf("ScanID(empty ctx) = %q, want \"\"", got)
	}
	ctx := WithScanID(context.Background(), "s-1")
	if got := ScanID(ctx); got != "s-1" {
		t.Errorf("ScanID = %q, want s-1", got)
	}
	// Empty scan id is a no-op so callers can unconditionally wrap.
	ctx2 := WithScanID(context.Background(), "")
	if got := ScanID(ctx2); got != "" {
		t.Errorf("ScanID(empty arg) = %q, want \"\"", got)
	}
	if ScanID(context.TODO()) != "" {
		t.Errorf("ScanID(empty context) should be empty string")
	}
}

// fakeClock is a monotonic test clock advanced manually.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
