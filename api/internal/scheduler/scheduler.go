package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jtb75/silkstrand/api/internal/model"
	"github.com/jtb75/silkstrand/api/internal/pubsub"
	"github.com/jtb75/silkstrand/api/internal/store"
)

// Dispatcher is the handoff point between a claimed scan_definition
// and the agent. The scheduler creates a `scans` row (via the store)
// and publishes a directive via Redis pub/sub; the existing
// AgentHandler.forwardDirective path enriches + sends the WSS message.
//
// Dispatch is idempotent per scheduler tick: if publish fails, the
// caller logs and moves on — next_run_at has already advanced, so the
// scan row becomes a one-off pending record that will be failed by the
// stuck-scan cleanup if the agent never picks it up. This matches
// ADR 007 D4 — operators accept "lose a tick" in exchange for
// crash-recovery simplicity.
type Dispatcher struct {
	Store  store.Store
	PubSub *pubsub.PubSub
}

// Scheduler polls for due scan_definitions and dispatches them. One
// goroutine per API process; `SELECT ... FOR UPDATE SKIP LOCKED`
// inside ClaimDueScanDefinitions ensures multiple instances never
// double-dispatch the same row.
type Scheduler struct {
	D        Dispatcher
	Interval time.Duration
}

// New builds a Scheduler with a default 30s tick per ADR 007 D4.
func New(s store.Store, ps *pubsub.PubSub) *Scheduler {
	return &Scheduler{
		D:        Dispatcher{Store: s, PubSub: ps},
		Interval: 30 * time.Second,
	}
}

// Run blocks until ctx is canceled, ticking every Interval. Errors
// from individual ticks are logged and never returned — the scheduler
// keeps running.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.Interval)
	defer t.Stop()
	slog.Info("scheduler.start", "interval", s.Interval.String())
	// Fire one immediate tick so locally-created due definitions don't
	// wait a full interval on boot.
	s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler.stop")
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

// Tick runs one scheduler cycle: claim due definitions, advance their
// next_run_at, and dispatch each.
func (s *Scheduler) Tick(ctx context.Context) {
	now := time.Now().UTC()
	claimed, err := s.D.Store.ClaimDueScanDefinitions(ctx, now, nextRun, 32)
	if err != nil {
		slog.Error("scheduler.claim", "error", err)
		return
	}
	if len(claimed) == 0 {
		return
	}
	slog.Info("scheduler.tick", "claimed", len(claimed))
	for _, d := range claimed {
		if err := s.D.Execute(ctx, d); err != nil {
			slog.Error("scheduler.dispatch",
				"definition", d.ID, "name", d.Name, "error", err)
			_ = s.D.Store.SetScanDefinitionLastRun(ctx, d.ID, now, "failed")
			continue
		}
		_ = s.D.Store.SetScanDefinitionLastRun(ctx, d.ID, now, "dispatched")
	}
}

// nextRun computes the next fire time for a cron expression. Called by
// the store-level claim transaction so advance + select are atomic.
func nextRun(schedule string, from time.Time) (time.Time, error) {
	if schedule == "" {
		return time.Time{}, errors.New("empty schedule")
	}
	c, err := ParseCron(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return c.Next(from)
}

// Execute materializes a scan row for the given definition and
// publishes a directive. Shared by the scheduler tick path and the
// POST /api/v1/scan-definitions/{id}/execute handler.
//
// Scope handling:
//   - asset_endpoint scope: scan the single endpoint. target_id comes
//     from a derived compliance-target row if one exists; for now we
//     dispatch with asset_endpoint_id set and target_id empty (agent
//     ignores target enrichment for discovery; compliance scans
//     against endpoints without a target are a post-P3 concern).
//   - cidr scope: upsert a targets row for (tenant, cidr) and dispatch
//     with that target_id. forwardDirective joins the target row to
//     populate target_type='cidr' + identifier=<cidr>, which naabu/httpx
//     consume as their input. Requires an agent_id on the definition —
//     a CIDR definition without an agent is a misconfiguration.
//   - collection scope: resolves endpoint ids and emits one scan per
//     endpoint (bounded by P3's naive resolver — every endpoint owned
//     by the tenant).
func (d Dispatcher) Execute(ctx context.Context, def model.ScanDefinition) error {
	switch def.ScopeKind {
	case model.ScanDefinitionScopeAssetEndpoint:
		if def.AssetEndpointID == nil {
			return fmt.Errorf("scope=asset_endpoint requires asset_endpoint_id")
		}
		return d.dispatchOne(ctx, def, def.AssetEndpointID, nil)
	case model.ScanDefinitionScopeCollection:
		if def.CollectionID == nil {
			return fmt.Errorf("scope=collection requires collection_id")
		}
		cctx := store.WithTenantID(ctx, def.TenantID)
		ids, err := d.Store.CollectionEndpointIDs(cctx, *def.CollectionID)
		if err != nil {
			return fmt.Errorf("resolving collection: %w", err)
		}
		if len(ids) == 0 {
			slog.Info("scheduler.collection_empty",
				"definition", def.ID, "collection", *def.CollectionID)
			return nil
		}
		for _, epID := range ids {
			epID := epID
			if err := d.dispatchOne(ctx, def, &epID, nil); err != nil {
				slog.Warn("scheduler.dispatch_one",
					"definition", def.ID, "endpoint", epID, "error", err)
			}
		}
		return nil
	case model.ScanDefinitionScopeCIDR:
		if def.CIDR == nil || *def.CIDR == "" {
			return fmt.Errorf("scope=cidr requires cidr")
		}
		if def.AgentID == nil {
			// No agent means the directive has nowhere to go.
			// forwardDirective still needs a target row, so fail loudly
			// here rather than produce an orphan scans row.
			return fmt.Errorf("scope=cidr requires agent_id")
		}
		targetID, err := d.Store.UpsertTargetByCIDR(ctx, def.TenantID, *def.CIDR, def.AgentID, "scheduled")
		if err != nil {
			return fmt.Errorf("upserting cidr target: %w", err)
		}
		return d.dispatchOne(ctx, def, nil, &targetID)
	}
	return fmt.Errorf("unknown scope_kind: %q", def.ScopeKind)
}

func (d Dispatcher) dispatchOne(ctx context.Context, def model.ScanDefinition, endpointID, targetID *string) error {
	scanType := model.ScanTypeCompliance
	if def.Kind == model.ScanDefinitionKindDiscovery {
		scanType = model.ScanTypeDiscovery
	}
	// Discovery definitions often have bundle_id=NULL (the UI doesn't
	// require one). The scan row and the WSS forwarder both need a valid
	// bundle FK, so default to the global discovery bundle.
	bundleID := def.BundleID
	if bundleID == nil && scanType == model.ScanTypeDiscovery {
		id := model.DiscoveryBundleID
		bundleID = &id
	}
	sc, err := d.Store.CreateScanForDefinition(ctx, store.CreateScanForDefinitionInput{
		TenantID:         def.TenantID,
		ScanDefinitionID: def.ID,
		AgentID:          def.AgentID,
		TargetID:         targetID,
		AssetEndpointID:  endpointID,
		BundleID:         bundleID,
		ScanType:         scanType,
	})
	if err != nil {
		return fmt.Errorf("creating scan: %w", err)
	}
	if def.AgentID == nil || d.PubSub == nil {
		slog.Info("scheduler.scan_created_without_dispatch",
			"scan", sc.ID, "reason", "no agent or pubsub")
		return nil
	}
	directive := pubsub.Directive{
		ScanID:   sc.ID,
		ScanType: scanType,
	}
	if bundleID != nil {
		directive.BundleID = *bundleID
	}
	if targetID != nil {
		directive.TargetID = *targetID
	}
	if err := d.PubSub.PublishDirective(ctx, *def.AgentID, directive); err != nil {
		return fmt.Errorf("publishing directive: %w", err)
	}
	slog.Info("scheduler.dispatched", "definition", def.ID, "scan", sc.ID, "agent", *def.AgentID)
	return nil
}
