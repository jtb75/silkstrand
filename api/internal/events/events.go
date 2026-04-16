// Package events implements the in-process event bus that powers the
// SSE framework described in docs/plans/scan-progress-and-sse.md.
//
// The bus is consumed by:
//   - scan_progress events emitted by the agent WSS handler per ADR 007 D4
//   - agent_log events per ADR 008
//   - audit.* events per ADR 005 (future)
//
// PR A ships the transport only — no emitters exist yet; later PRs plug
// publishers into Bus.Publish and rely on this package's subscribers to
// fan out over the /api/v1/events/stream SSE endpoint.
package events

import (
	"context"
	"encoding/json"
	"time"
)

// Event is the envelope shared across all event kinds. The shape matches
// the ADR 005 audit-event shape so audit events can eventually flow
// through the same bus without schema drift.
type Event struct {
	// Kind is a dotted namespace (e.g. "scan_progress", "agent_log",
	// "audit.credential.fetch"). Consumers filter by prefix or exact match.
	Kind string `json:"kind"`

	// ResourceType is the domain object this event is about
	// (e.g. "scan", "agent", "asset", "rule"). Optional for system events.
	ResourceType string `json:"resource_type,omitempty"`

	// ResourceID is the UUID (as text) of the resource. Optional.
	ResourceID string `json:"resource_id,omitempty"`

	// TenantID scopes the event to a tenant. Required — the bus refuses
	// to publish events without one so tenant isolation can't be bypassed
	// by accident. Not sent on the wire (the SSE endpoint already
	// tenant-scopes its subscriptions).
	TenantID string `json:"-"`

	// OccurredAt is the RFC3339 timestamp of when the event happened.
	OccurredAt time.Time `json:"occurred_at"`

	// Payload is kind-specific JSON. Keep this as RawMessage so the bus
	// doesn't have to know about every downstream payload shape.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Filter narrows the stream of events a subscriber receives. TenantID is
// required and is enforced by the middleware that mints the filter; the
// other fields are optional.
type Filter struct {
	// TenantID is required; events with a different tenant are never
	// delivered to this subscriber.
	TenantID string

	// Kinds optionally restricts the subscription to a set of kinds.
	// Empty slice = all kinds allowed.
	Kinds []string

	// ResourceType optionally restricts by resource type (e.g. "agent").
	ResourceType string

	// ResourceID optionally restricts by resource id.
	ResourceID string

	// ScanID reads payload.scan_id; useful for the "per-scan console"
	// agent_log subscription pattern. Empty = no filter.
	ScanID string
}

// matches returns true if the event satisfies this filter.
func (f Filter) matches(e Event) bool {
	if f.TenantID != "" && e.TenantID != f.TenantID {
		return false
	}
	if len(f.Kinds) > 0 {
		ok := false
		for _, k := range f.Kinds {
			if k == e.Kind {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.ResourceType != "" && e.ResourceType != f.ResourceType {
		return false
	}
	if f.ResourceID != "" && e.ResourceID != f.ResourceID {
		return false
	}
	if f.ScanID != "" {
		// Peek into payload for scan_id. Payload may be absent for some
		// kinds; treat that as a non-match so the filter stays strict.
		if len(e.Payload) == 0 {
			return false
		}
		var probe struct {
			ScanID string `json:"scan_id"`
		}
		if err := json.Unmarshal(e.Payload, &probe); err != nil {
			return false
		}
		if probe.ScanID != f.ScanID {
			return false
		}
	}
	return true
}

// Bus is the publish/subscribe surface for cross-component events.
// Today's implementation is in-process; the interface leaves the door
// open to a Redis-backed implementation when we need multi-pod fan-out.
type Bus interface {
	// Publish delivers an event to all matching subscribers. Non-blocking:
	// if any subscriber's buffer is full, its event is dropped and a
	// counter is incremented. The publisher never stalls.
	Publish(ctx context.Context, e Event) error

	// Subscribe registers a subscriber with the given filter. Returns a
	// receive-only channel of events and a cancel function that the
	// caller MUST invoke when done. The channel is closed by cancel;
	// reads after cancel will return the zero value.
	Subscribe(ctx context.Context, filter Filter) (<-chan Event, func())
}
