package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryBusPublishSubscribe(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub := bus.Subscribe(ctx, Filter{TenantID: "t-1"})
	defer unsub()

	want := Event{
		Kind:       "scan_progress",
		TenantID:   "t-1",
		OccurredAt: time.Now(),
	}
	if err := bus.Publish(ctx, want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-ch:
		if got.Kind != want.Kind {
			t.Errorf("Kind: got %q want %q", got.Kind, want.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestMemoryBusMissingTenant(t *testing.T) {
	bus := NewMemoryBus()
	if err := bus.Publish(context.Background(), Event{Kind: "k"}); err != ErrMissingTenant {
		t.Errorf("Publish without tenant: got %v want %v", err, ErrMissingTenant)
	}
}

func TestMemoryBusTenantIsolation(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chA, unsubA := bus.Subscribe(ctx, Filter{TenantID: "tenant-a"})
	defer unsubA()
	chB, unsubB := bus.Subscribe(ctx, Filter{TenantID: "tenant-b"})
	defer unsubB()

	// Publish one event per tenant
	_ = bus.Publish(ctx, Event{Kind: "k", TenantID: "tenant-a"})
	_ = bus.Publish(ctx, Event{Kind: "k", TenantID: "tenant-b"})

	// A should see exactly the tenant-a event
	select {
	case e := <-chA:
		if e.TenantID != "tenant-a" {
			t.Errorf("chA received tenant=%q want tenant-a", e.TenantID)
		}
	case <-time.After(time.Second):
		t.Fatal("chA: timed out")
	}
	// And nothing more
	select {
	case e := <-chA:
		t.Errorf("chA: unexpected crossover event tenant=%q", e.TenantID)
	case <-time.After(50 * time.Millisecond):
	}

	// B should see exactly the tenant-b event
	select {
	case e := <-chB:
		if e.TenantID != "tenant-b" {
			t.Errorf("chB received tenant=%q want tenant-b", e.TenantID)
		}
	case <-time.After(time.Second):
		t.Fatal("chB: timed out")
	}
}

func TestMemoryBusFilterKinds(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub := bus.Subscribe(ctx, Filter{TenantID: "t-1", Kinds: []string{"scan_progress"}})
	defer unsub()

	_ = bus.Publish(ctx, Event{Kind: "agent_log", TenantID: "t-1"})
	_ = bus.Publish(ctx, Event{Kind: "scan_progress", TenantID: "t-1"})

	select {
	case e := <-ch:
		if e.Kind != "scan_progress" {
			t.Errorf("Kind: got %q want scan_progress", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	// No second event expected
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event kind=%q", e.Kind)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMemoryBusFilterResourceID(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub := bus.Subscribe(ctx, Filter{TenantID: "t-1", ResourceType: "agent", ResourceID: "a-1"})
	defer unsub()

	_ = bus.Publish(ctx, Event{Kind: "agent_log", TenantID: "t-1", ResourceType: "agent", ResourceID: "a-2"})
	_ = bus.Publish(ctx, Event{Kind: "agent_log", TenantID: "t-1", ResourceType: "scan", ResourceID: "a-1"})
	_ = bus.Publish(ctx, Event{Kind: "agent_log", TenantID: "t-1", ResourceType: "agent", ResourceID: "a-1"})

	select {
	case e := <-ch:
		if e.ResourceType != "agent" || e.ResourceID != "a-1" {
			t.Errorf("got (type=%q id=%q), want (agent, a-1)", e.ResourceType, e.ResourceID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMemoryBusFilterScanID(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsub := bus.Subscribe(ctx, Filter{TenantID: "t-1", ScanID: "scan-1"})
	defer unsub()

	p1, _ := json.Marshal(map[string]string{"scan_id": "scan-2", "stage": "naabu"})
	p2, _ := json.Marshal(map[string]string{"scan_id": "scan-1", "stage": "naabu"})
	p3, _ := json.Marshal(map[string]string{"stage": "naabu"}) // no scan_id

	_ = bus.Publish(ctx, Event{Kind: "scan_progress", TenantID: "t-1", Payload: p1})
	_ = bus.Publish(ctx, Event{Kind: "scan_progress", TenantID: "t-1", Payload: p3})
	_ = bus.Publish(ctx, Event{Kind: "scan_progress", TenantID: "t-1", Payload: p2})

	select {
	case e := <-ch:
		var probe struct {
			ScanID string `json:"scan_id"`
		}
		if err := json.Unmarshal(e.Payload, &probe); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if probe.ScanID != "scan-1" {
			t.Errorf("scan_id: got %q want scan-1", probe.ScanID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestMemoryBusSlowSubscriberDrops(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe but deliberately never drain.
	_, unsub := bus.Subscribe(ctx, Filter{TenantID: "t-1"})
	defer unsub()

	// Fill the buffer and then some.
	over := subscriberBufferSize + 5
	for i := 0; i < over; i++ {
		_ = bus.Publish(ctx, Event{Kind: "k", TenantID: "t-1"})
	}

	if got := bus.DroppedCount(); got < 5 {
		t.Errorf("DroppedCount: got %d want >= 5", got)
	}
}

func TestMemoryBusContextCancelUnsubs(t *testing.T) {
	bus := NewMemoryBus()
	ctx, cancel := context.WithCancel(context.Background())
	_, _ = bus.Subscribe(ctx, Filter{TenantID: "t-1"})

	if got := bus.SubscriberCount(); got != 1 {
		t.Fatalf("SubscriberCount before cancel: got %d want 1", got)
	}
	cancel()

	// The auto-unsub goroutine races with the assertion; poll briefly.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if bus.SubscriberCount() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("SubscriberCount after ctx cancel: got %d want 0", bus.SubscriberCount())
}
