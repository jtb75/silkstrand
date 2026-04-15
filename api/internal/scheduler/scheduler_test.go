package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// fakeStore implements the bits of store.Store that the scheduler touches.
// Only the methods under test need real behavior; the rest are stubs so
// the type satisfies the interface.
type fakeStore struct {
	store.Store
	claimed     []model.ScanDefinition
	nextRunAt   map[string]time.Time
	createCalls int
	createErr   error
	lastRun     map[string]string
}

func (f *fakeStore) ClaimDueScanDefinitions(ctx context.Context, now time.Time, next func(string, time.Time) (time.Time, error), limit int) ([]model.ScanDefinition, error) {
	// Simulate the SQL path: compute next_run_at for each claimed row via `next`.
	if f.nextRunAt == nil {
		f.nextRunAt = map[string]time.Time{}
	}
	for _, d := range f.claimed {
		s := ""
		if d.Schedule != nil {
			s = *d.Schedule
		}
		n, err := next(s, now)
		if err == nil {
			f.nextRunAt[d.ID] = n
		}
	}
	out := f.claimed
	f.claimed = nil
	return out, nil
}

func (f *fakeStore) CreateScanForDefinition(ctx context.Context, in store.CreateScanForDefinitionInput) (*model.Scan, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &model.Scan{ID: "scan-1", TenantID: in.TenantID, ScanDefinitionID: &in.ScanDefinitionID}, nil
}

func (f *fakeStore) CollectionEndpointIDs(ctx context.Context, id string) ([]string, error) {
	return []string{"ep-1"}, nil
}

func (f *fakeStore) SetScanDefinitionLastRun(ctx context.Context, id string, at time.Time, status string) error {
	if f.lastRun == nil {
		f.lastRun = map[string]string{}
	}
	f.lastRun[id] = status
	return nil
}

// TestTickCrashRecovery — if dispatch (CreateScanForDefinition) fails,
// next_run_at has still been advanced inside ClaimDueScanDefinitions
// so the definition does not wedge in a perpetually-due state, and
// last_run_status records the failure. This matches ADR 007 D4's
// "lose a tick, not a definition" invariant.
func TestTickCrashRecovery(t *testing.T) {
	cron := "*/5 * * * *"
	now := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	endpointID := "ep-1"
	def := model.ScanDefinition{
		ID:              "def-1",
		TenantID:        "t-1",
		Kind:            model.ScanDefinitionKindCompliance,
		ScopeKind:       model.ScanDefinitionScopeAssetEndpoint,
		AssetEndpointID: &endpointID,
		Schedule:        &cron,
		Enabled:         true,
		NextRunAt:       &now,
	}
	f := &fakeStore{
		claimed:   []model.ScanDefinition{def},
		createErr: errors.New("boom"),
	}
	s := &Scheduler{D: Dispatcher{Store: f}, Interval: time.Minute}
	s.Tick(context.Background())

	if f.createCalls != 1 {
		t.Fatalf("CreateScanForDefinition calls: got %d want 1", f.createCalls)
	}
	gotNext, ok := f.nextRunAt["def-1"]
	if !ok {
		t.Fatal("next_run_at never advanced; scheduler would re-fire forever")
	}
	if !gotNext.After(now) {
		t.Errorf("next_run_at=%v did not advance past now=%v", gotNext, now)
	}
	if got := f.lastRun["def-1"]; got != "failed" {
		t.Errorf("last_run_status: got %q want 'failed'", got)
	}
}
