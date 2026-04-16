package logstream

import (
	"context"
	"errors"
	"log/slog"
)

// MultiHandler fans an slog.Record out to several handlers. stdlib slog
// does not ship a multiplexer, so this is a ~30-line implementation —
// just enough for the agent's "stdout + tunnel" split per ADR 008 D1.
//
// Enabled returns true if ANY downstream handler is Enabled; per-record
// filtering still happens in each Handle call so the tunnel handler can
// drop debug while stdout keeps it.
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMulti returns a MultiHandler that dispatches to the given handlers
// in order. Nil entries are dropped silently.
func NewMulti(handlers ...slog.Handler) slog.Handler {
	live := make([]slog.Handler, 0, len(handlers))
	for _, h := range handlers {
		if h != nil {
			live = append(live, h)
		}
	}
	return &MultiHandler{handlers: live}
}

// Enabled reports whether at least one downstream handler wants records
// at the given level.
func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches to every handler that is Enabled for this record.
// Errors from individual handlers are collected and returned joined —
// losing a tunnel emit must not silently swallow a stdout log error.
func (m *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		// Clone so each handler sees an independent record — some
		// implementations mutate attrs via AddAttrs.
		if err := h.Handle(ctx, r.Clone()); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// WithAttrs returns a new MultiHandler where each downstream handler has
// been enriched with the given attrs.
func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: out}
}

// WithGroup returns a new MultiHandler where each downstream handler has
// been entered into the named group.
func (m *MultiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		out[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: out}
}
