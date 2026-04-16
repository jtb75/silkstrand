package logstream

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jtb75/silkstrand/agent/internal/tunnel"
)

// Message-type constant for tunnel log records. Keep the literal in one
// place so the server handler and any future tooling grep cleanly. The
// tunnel package doesn't own it since it's a logstream concept.
const TypeAgentLog = "agent_log"

// Rate limit defaults per ADR 008 D6. A noisy bundle shouldn't be able
// to drown the tunnel or the UI console. 50/s steady-state, burst 100 —
// comfortably above observed normal operation (~1/s, ~10/s at scan
// entry).
const (
	defaultRefillPerSec = 50
	defaultBurst        = 100

	// throttleSummaryWindow bounds how often we emit an
	// agent_log.throttled summary after drops start. ADR 008 D6.
	throttleSummaryWindow = 5 * time.Second
)

// tunnelSender is the minimal surface TunnelHandler needs from the real
// tunnel.Tunnel. Defined as an interface so unit tests can plug in an
// in-memory mock without standing up a websocket.
type tunnelSender interface {
	Send(msg tunnel.Message)
}

// TunnelHandler is an slog.Handler that publishes info+ records over the
// agent's existing WSS tunnel as {type: "agent_log"} messages (ADR 008
// D1/D2/D6/D7). The record payload follows the ADR 008 D3 envelope:
//
//	{ "level": "INFO", "msg": "...", "scan_id": "...", "attrs": {...} }
//
// The handler is safe for concurrent use.
type TunnelHandler struct {
	tun tunnelSender

	// preGroupAttrs are attrs accumulated via WithAttrs BEFORE any
	// WithGroup call — they land in payload.attrs at the top level.
	// groupChain records the stack of WithGroup calls and the attrs
	// attached after each one, so dotted prefixes accumulate correctly
	// (slog guarantees "attrs attached after WithGroup(G) nest under G").
	preGroupAttrs []slog.Attr
	groupChain    []groupFrame

	// Token-bucket rate limiter state. Hand-rolled to keep the agent
	// dependency footprint small (no golang.org/x/time/rate).
	limiter *tokenBucket

	// dropped counts records thrown away by the rate limiter since the
	// last throttle summary. Read+reset atomically in maybeEmitThrottled.
	dropped atomic.Int64

	// throttleMu serializes the decision to emit a summary, so two
	// goroutines that both notice drops can't double-emit.
	throttleMu       sync.Mutex
	lastSummaryEmit  time.Time
	throttleInFlight bool

	// nowFunc is a seam for tests; production path uses time.Now.
	nowFunc func() time.Time
}

// TunnelHandlerOption tunes a TunnelHandler. Only a handful of knobs
// today — exposed primarily for tests and future backoff tuning.
type TunnelHandlerOption func(*TunnelHandler)

// WithRateLimit overrides the default token-bucket settings.
// refillPerSec <= 0 disables rate limiting entirely.
func WithRateLimit(refillPerSec int, burst int) TunnelHandlerOption {
	return func(h *TunnelHandler) {
		h.limiter = newTokenBucket(refillPerSec, burst, h.nowFunc)
	}
}

// WithNow injects a clock for deterministic tests.
func WithNow(f func() time.Time) TunnelHandlerOption {
	return func(h *TunnelHandler) {
		h.nowFunc = f
		if h.limiter != nil {
			h.limiter.nowFunc = f
		}
	}
}

// NewTunnelHandler builds a TunnelHandler that ships records via tun.
// The caller is responsible for installing it (typically through
// NewMulti alongside a stdout JSON handler — see ADR 008 D1).
func NewTunnelHandler(tun tunnelSender, opts ...TunnelHandlerOption) *TunnelHandler {
	h := &TunnelHandler{
		tun:     tun,
		nowFunc: time.Now,
	}
	h.limiter = newTokenBucket(defaultRefillPerSec, defaultBurst, h.nowFunc)
	for _, o := range opts {
		o(h)
	}
	return h
}

// Enabled implements slog.Handler. Per ADR 008 D2 the tunnel stream is
// info-and-above only; debug stays on the local file so the tunnel + UI
// aren't drowned by the highest-volume, lowest-value category.
func (h *TunnelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

// Handle implements slog.Handler. Builds the ADR 008 D3 envelope payload,
// consults the rate limiter, and either emits or bumps the drop counter.
// Never blocks: tunnel.Send is itself non-blocking (drops on full buffer).
func (h *TunnelHandler) Handle(ctx context.Context, r slog.Record) error {
	// Defense in depth — callers should already have gated by Enabled.
	if r.Level < slog.LevelInfo {
		return nil
	}

	attrs := h.collectAttrs(r)

	payload := map[string]any{
		"level": r.Level.String(),
		"msg":   r.Message,
	}
	if scanID := ScanID(ctx); scanID != "" {
		payload["scan_id"] = scanID
	}
	if len(attrs) > 0 {
		payload["attrs"] = attrs
	}

	if !h.limiter.allow() {
		h.dropped.Add(1)
		h.maybeEmitThrottled()
		return nil
	}

	h.send(TypeAgentLog, payload)
	h.maybeEmitThrottled()
	return nil
}

// groupFrame captures a WithGroup boundary plus the attrs added after
// it. Dotted prefix = join(groupChain[:i+1].name, ".").
type groupFrame struct {
	name  string
	attrs []slog.Attr
}

// WithAttrs implements slog.Handler. Attrs land in the current group
// (or at the top level if no group is active).
func (h *TunnelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clone := h.shallow()
	if len(clone.groupChain) == 0 {
		clone.preGroupAttrs = append(append([]slog.Attr{}, h.preGroupAttrs...), attrs...)
		return clone
	}
	// Copy the chain and append attrs to the innermost frame.
	clone.groupChain = make([]groupFrame, len(h.groupChain))
	copy(clone.groupChain, h.groupChain)
	last := &clone.groupChain[len(clone.groupChain)-1]
	last.attrs = append(append([]slog.Attr{}, last.attrs...), attrs...)
	return clone
}

// WithGroup implements slog.Handler. Opens a new group frame; subsequent
// attrs and the record's own attrs will nest under the dotted prefix.
func (h *TunnelHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := h.shallow()
	clone.groupChain = append(append([]groupFrame{}, h.groupChain...), groupFrame{name: name})
	return clone
}

// shallow returns a copy that shares mutable state (limiter, counters,
// tunnel) but owns its own attrs/group slices. WithAttrs / WithGroup
// require this so derived loggers don't contaminate the base.
func (h *TunnelHandler) shallow() *TunnelHandler {
	return &TunnelHandler{
		tun:             h.tun,
		preGroupAttrs:   h.preGroupAttrs,
		groupChain:      h.groupChain,
		limiter:         h.limiter,
		nowFunc:         h.nowFunc,
		lastSummaryEmit: h.lastSummaryEmit,
	}
}

// collectAttrs flattens the handler's accumulated attrs plus the record's
// attrs into a flat map. Groups prefix nested keys with "<g1.g2>.".
func (h *TunnelHandler) collectAttrs(r slog.Record) map[string]any {
	out := map[string]any{}
	for _, a := range h.preGroupAttrs {
		addAttr(out, "", a)
	}
	// Walk each group frame applying its prefix to its attrs.
	prefix := ""
	for i, frame := range h.groupChain {
		if i == 0 {
			prefix = frame.name + "."
		} else {
			prefix = prefix + frame.name + "."
		}
		for _, a := range frame.attrs {
			addAttr(out, prefix, a)
		}
	}
	// Record-level attrs nest under the deepest group.
	r.Attrs(func(a slog.Attr) bool {
		addAttr(out, prefix, a)
		return true
	})
	return out
}

func addAttr(out map[string]any, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if a.Value.Kind() == slog.KindGroup {
		gprefix := prefix + a.Key + "."
		for _, sub := range a.Value.Group() {
			addAttr(out, gprefix, sub)
		}
		return
	}
	out[prefix+a.Key] = a.Value.Any()
}

// maybeEmitThrottled fires a single agent_log.throttled summary event
// when drops have accumulated and the 5s window has elapsed. The summary
// itself bypasses the limiter — the whole point is to be reliably heard.
func (h *TunnelHandler) maybeEmitThrottled() {
	drops := h.dropped.Load()
	if drops == 0 {
		return
	}

	h.throttleMu.Lock()
	if h.throttleInFlight {
		h.throttleMu.Unlock()
		return
	}
	now := h.nowFunc()
	if !h.lastSummaryEmit.IsZero() && now.Sub(h.lastSummaryEmit) < throttleSummaryWindow {
		h.throttleMu.Unlock()
		return
	}
	h.throttleInFlight = true
	h.throttleMu.Unlock()

	// Snapshot + zero the counter. A burst that lands between the load
	// and the swap is attributed to the next window — acceptable; the
	// counter is "at least N dropped", not "exactly".
	count := h.dropped.Swap(0)

	h.send(TypeAgentLog, map[string]any{
		"level": slog.LevelWarn.String(),
		"msg":   "agent_log.throttled",
		"attrs": map[string]any{
			"dropped": count,
			"window":  throttleSummaryWindow.String(),
		},
	})

	h.throttleMu.Lock()
	h.lastSummaryEmit = now
	h.throttleInFlight = false
	h.throttleMu.Unlock()
}

// send marshals the payload and hands it to the tunnel. Failures to
// marshal (shouldn't happen for the shapes we construct) are swallowed —
// losing a log line is strictly preferable to crashing the agent.
func (h *TunnelHandler) send(msgType string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.tun.Send(tunnel.Message{Type: msgType, Payload: raw})
}

// ---------------------------------------------------------------------
// tokenBucket — minimal rate limiter so we don't pull in x/time/rate.
// ---------------------------------------------------------------------

type tokenBucket struct {
	mu           sync.Mutex
	refillPerSec float64
	burst        float64
	tokens       float64
	last         time.Time
	nowFunc      func() time.Time
	disabled     bool
}

func newTokenBucket(refillPerSec, burst int, nowFunc func() time.Time) *tokenBucket {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	if refillPerSec <= 0 {
		return &tokenBucket{disabled: true, nowFunc: nowFunc}
	}
	return &tokenBucket{
		refillPerSec: float64(refillPerSec),
		burst:        float64(burst),
		tokens:       float64(burst),
		last:         nowFunc(),
		nowFunc:      nowFunc,
	}
}

// allow returns true if a token was available (and consumes it), false
// if the bucket was empty.
func (b *tokenBucket) allow() bool {
	if b == nil || b.disabled {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.nowFunc()
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min64(b.burst, b.tokens+elapsed*b.refillPerSec)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
