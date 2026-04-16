package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// subscriberBufferSize is the per-subscriber channel buffer. Small enough
// that a slow subscriber gets dropped quickly instead of wedging the
// publisher; large enough that normal bursty traffic (e.g. a rule fire
// that emits a handful of events in rapid succession) doesn't spuriously
// drop.
const subscriberBufferSize = 64

// MemoryBus is an in-process implementation of Bus. Safe for concurrent
// publish/subscribe. Tenant isolation is enforced at filter-match time —
// events with tenant A are never delivered to a subscriber with filter
// TenantID=B.
//
// Cross-pod fan-out is explicitly out of scope per the PR A plan; with
// N=1 API instance the in-process path is fine, and the Bus interface
// leaves room to swap in a Redis-backed impl later.
type MemoryBus struct {
	mu      sync.RWMutex
	nextID  uint64
	subs    map[uint64]*subscription
	dropped atomic.Uint64 // monotonic counter of dropped events
}

type subscription struct {
	filter Filter
	ch     chan Event
	// closed guards against double-close in cancel().
	closed atomic.Bool
	once   sync.Once
}

// NewMemoryBus constructs an empty bus.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{subs: make(map[uint64]*subscription)}
}

// ErrMissingTenant is returned by Publish when an event lacks a TenantID.
// Defensive guard: tenant isolation is a correctness invariant, not an
// optional decoration.
var ErrMissingTenant = errors.New("events: event missing tenant_id")

// Publish delivers the event to every matching subscriber. Runs in O(N)
// over subscribers holding only a read lock; per-subscriber send is
// non-blocking via select/default so one slow consumer can't stall
// emitters or other consumers.
func (b *MemoryBus) Publish(_ context.Context, e Event) error {
	if e.TenantID == "" {
		return ErrMissingTenant
	}

	b.mu.RLock()
	// Snapshot matching subscribers under the read lock, then release
	// before sending — keeps the lock window short and avoids a publisher
	// holding the lock while waiting on a full channel.
	var targets []*subscription
	for _, s := range b.subs {
		if s.filter.matches(e) {
			targets = append(targets, s)
		}
	}
	b.mu.RUnlock()

	for _, s := range targets {
		if s.closed.Load() {
			continue
		}
		select {
		case s.ch <- e:
		default:
			// Slow subscriber; drop + record. Counter is monotonic so the
			// SSE handler can emit a one-line warn on cancel if the
			// dropped count moved since Subscribe() — useful signal
			// without per-event log spam.
			b.dropped.Add(1)
		}
	}
	return nil
}

// Subscribe registers a subscriber. The returned channel is closed when
// the caller invokes the returned cancel function.
//
// Implementation note: the caller's ctx is watched; if it is canceled
// before the caller calls cancel, the subscription is torn down
// automatically. That's important for HTTP handlers where the client
// disconnect cancels ctx and we must free the slot.
func (b *MemoryBus) Subscribe(ctx context.Context, filter Filter) (<-chan Event, func()) {
	sub := &subscription{
		filter: filter,
		ch:     make(chan Event, subscriberBufferSize),
	}

	b.mu.Lock()
	b.nextID++
	id := b.nextID
	b.subs[id] = sub
	b.mu.Unlock()

	cancel := func() {
		sub.once.Do(func() {
			sub.closed.Store(true)
			b.mu.Lock()
			delete(b.subs, id)
			b.mu.Unlock()
			close(sub.ch)
		})
	}

	// Auto-unsubscribe on ctx cancel. Goroutine exits on the first
	// cancel — explicit caller cancel or ctx done, whichever fires first.
	go func() {
		<-ctx.Done()
		cancel()
	}()

	return sub.ch, cancel
}

// DroppedCount returns the monotonic count of events dropped to slow
// subscribers. Intended for metrics + debug log lines, not for control
// flow.
func (b *MemoryBus) DroppedCount() uint64 {
	return b.dropped.Load()
}

// SubscriberCount returns the current number of active subscribers.
// Used by tests; also handy for a /metrics-style debug endpoint if we
// ever add one.
func (b *MemoryBus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
