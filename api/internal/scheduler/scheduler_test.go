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
	claimed      []model.ScanDefinition
	nextRunAt    map[string]time.Time
	createCalls  int
	createErr    error
	lastRun      map[string]string
	cidrUpserts  []cidrUpsert
	createInputs []store.CreateScanForDefinitionInput
}

type cidrUpsert struct {
	TenantID    string
	CIDR        string
	AgentID     *string
	Environment string
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
	f.createInputs = append(f.createInputs, in)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &model.Scan{ID: "scan-1", TenantID: in.TenantID, ScanDefinitionID: &in.ScanDefinitionID}, nil
}

func (f *fakeStore) UpsertTargetByCIDR(ctx context.Context, tenantID, cidr string, agentID *string, environment string) (string, error) {
	f.cidrUpserts = append(f.cidrUpserts, cidrUpsert{
		TenantID: tenantID, CIDR: cidr, AgentID: agentID, Environment: environment,
	})
	return "target-cidr-1", nil
}

func (f *fakeStore) CollectionEndpointIDs(ctx context.Context, id string) ([]string, error) {
	return []string{"ep-1"}, nil
}

func (f *fakeStore) AgentHasRunningScan(ctx context.Context, agentID string) (bool, error) {
	return false, nil
}

func (f *fakeStore) UpdateScanStatus(ctx context.Context, scanID, status string) error {
	return nil
}

func (f *fakeStore) FailStaleQueuedScans(ctx context.Context, maxAge time.Duration) (int, error) {
	return 0, nil
}

func (f *fakeStore) OldestQueuedScanForAgent(ctx context.Context, agentID string) (*model.Scan, error) {
	return nil, nil
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

// TestExecuteCIDRScope verifies CIDR-scope scan_definitions materialize
// a targets row via UpsertTargetByCIDR and dispatch a scan referencing
// that target_id. Without this wiring the CIDR-scope branch silently
// skipped, which was the pre-fix bug this PR closes.
func TestExecuteCIDRScope(t *testing.T) {
	cidr := "192.168.0.0/24"
	agent := "agent-1"
	bundle := "bundle-discovery"
	def := model.ScanDefinition{
		ID:        "def-cidr",
		TenantID:  "t-1",
		Kind:      model.ScanDefinitionKindDiscovery,
		ScopeKind: model.ScanDefinitionScopeCIDR,
		CIDR:      &cidr,
		AgentID:   &agent,
		BundleID:  &bundle,
		Enabled:   true,
	}
	f := &fakeStore{}
	// PubSub=nil means dispatchOne creates the scan row but skips
	// PublishDirective — that's fine for this test; we're checking the
	// store side of the wiring. The agent-connected path is exercised
	// by the e2e smoketest.
	d := Dispatcher{Store: f}
	if err := d.Execute(context.Background(), def); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(f.cidrUpserts) != 1 {
		t.Fatalf("UpsertTargetByCIDR calls: got %d want 1", len(f.cidrUpserts))
	}
	up := f.cidrUpserts[0]
	if up.TenantID != "t-1" || up.CIDR != cidr {
		t.Errorf("UpsertTargetByCIDR tenant/cidr: got %q/%q", up.TenantID, up.CIDR)
	}
	if up.AgentID == nil || *up.AgentID != agent {
		t.Errorf("UpsertTargetByCIDR agent: got %v want %q", up.AgentID, agent)
	}
	if f.createCalls != 1 {
		t.Fatalf("CreateScanForDefinition calls: got %d want 1", f.createCalls)
	}
	in := f.createInputs[0]
	if in.TargetID == nil || *in.TargetID != "target-cidr-1" {
		t.Errorf("scan.target_id: got %v want target-cidr-1", in.TargetID)
	}
	if in.ScanType != model.ScanTypeDiscovery {
		t.Errorf("scan.scan_type: got %q want %q", in.ScanType, model.ScanTypeDiscovery)
	}
}

// TestExecuteCIDRScopeMissingCIDR — the scan_definitions CHECK enforces
// a non-null cidr for scope=cidr, but the dispatcher should also refuse
// to create an orphan scan row if the value is somehow empty.
func TestExecuteCIDRScopeMissingCIDR(t *testing.T) {
	agent := "agent-1"
	def := model.ScanDefinition{
		ID:        "def-cidr",
		TenantID:  "t-1",
		Kind:      model.ScanDefinitionKindDiscovery,
		ScopeKind: model.ScanDefinitionScopeCIDR,
		CIDR:      nil,
		AgentID:   &agent,
	}
	f := &fakeStore{}
	d := Dispatcher{Store: f}
	if err := d.Execute(context.Background(), def); err == nil {
		t.Fatal("Execute: expected error for missing cidr, got nil")
	}
	if len(f.cidrUpserts) != 0 {
		t.Errorf("UpsertTargetByCIDR should not be called; got %d", len(f.cidrUpserts))
	}
	if f.createCalls != 0 {
		t.Errorf("CreateScanForDefinition should not be called; got %d", f.createCalls)
	}
}

// TestExecuteCIDRScopeMissingAgent — a CIDR-scope definition without an
// agent cannot dispatch (forwardDirective needs an agent to send to).
// Refuse early rather than create a zombie scan row.
func TestExecuteCIDRScopeMissingAgent(t *testing.T) {
	cidr := "10.0.0.0/24"
	def := model.ScanDefinition{
		ID:        "def-cidr",
		TenantID:  "t-1",
		Kind:      model.ScanDefinitionKindDiscovery,
		ScopeKind: model.ScanDefinitionScopeCIDR,
		CIDR:      &cidr,
		AgentID:   nil,
	}
	f := &fakeStore{}
	d := Dispatcher{Store: f}
	if err := d.Execute(context.Background(), def); err == nil {
		t.Fatal("Execute: expected error for missing agent, got nil")
	}
	if len(f.cidrUpserts) != 0 {
		t.Errorf("UpsertTargetByCIDR should not be called; got %d", len(f.cidrUpserts))
	}
}
