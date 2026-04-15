# ADR 007: Unified findings + scan definitions and scheduler

**Status:** Accepted (P4–P5 shipped v0.1.49)
**Date:** 2026-04-15
**Related:** [ADR 003](./003-recon-pipeline.md) (recon pipeline — produces today's
cve list on `discovered_assets`), [ADR 004](./004-credential-resolver.md)
(credential sources — referenced by scan definitions), [ADR 005](./005-audit-events.md)
(audit surface — scheduler ticks will emit audit events), [ADR 006](./006-asset-first-data-model.md)
(asset / endpoint split, collections — this ADR reads endpoint ids from that
model), roadmap plan (approved) covering all phases, UI shape at
`docs/plans/ui-shape.md`.

---

## Context

ADR 006 locked in the asset-first data model for Phases 1–3 (assets,
asset_endpoints, collections, rules-reference-collections). This ADR covers
Phases 4–5 of the roadmap: the findings model and the scan scheduler.

Four problems are addressed here:

1. **Findings are denormalized and source-less.** Nuclei CVE hits today land
   as a JSONB array on `discovered_assets.cves`. Compliance-bundle check
   results land in a separate `scan_results` table. There's no shared shape
   and no way to ask "every finding across both sources for asset X." You
   can't attach provenance metadata (template id, matched-at URL) without
   extending one of two ad-hoc schemas.

2. **Scans have no schedule.** Every scan today is either a manual click
   from the UI or a one-shot fan-out from a rule. Recurring discovery —
   arguably the core value of the product — requires a cron somewhere
   outside our own system. "Run CIS-postgres against every new postgres we
   discover, every 24 hours" isn't expressible.

3. **Targets and scans are tangled.** A scan today must have either a
   `target_id` OR a `parent_one_shot_id` (plus an `inline_predicate` that
   bypasses targets entirely). That's three incompatible code paths through
   `scan.go:Create`, `one_shot.go`, and the agent dispatch logic. Scheduling
   layered on top of that gets worse.

4. **"One-shot scan" is a weird name.** Today it means "fan out this
   bundle across the matching endpoints and auto-clean the spawned
   targets." Post-ADR-006 we can drop the "one-shot" framing entirely:
   every run is an execution of a *definition*, and the difference is just
   whether the definition's schedule is null (manual) or a cron (recurring).

## Decisions

### D1. One unified `findings` table

```sql
CREATE TABLE findings (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    asset_endpoint_id UUID NOT NULL REFERENCES asset_endpoints(id) ON DELETE CASCADE,
    scan_id UUID,                   -- nullable; discovery findings pre-exist any scan
    source_kind TEXT NOT NULL,      -- 'network_vuln' | 'network_compliance' | 'bundle_compliance'
    source TEXT NOT NULL,           -- 'nuclei' | 'httpx' | 'cis-postgresql-16' | ...
    source_id TEXT,                 -- template id, cve id, control id
    cve_id TEXT,                    -- nullable; only set when applicable
    severity TEXT,                  -- 'info' | 'low' | 'medium' | 'high' | 'critical'
    title TEXT NOT NULL,
    status TEXT NOT NULL,           -- 'open' | 'resolved' | 'suppressed'
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    remediation TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    PRIMARY KEY (id, first_seen)
) PARTITION BY RANGE (first_seen);

CREATE INDEX idx_findings_endpoint ON findings (asset_endpoint_id, status, severity);
CREATE INDEX idx_findings_source ON findings (source_kind, source, source_id);
CREATE INDEX idx_findings_cve ON findings (cve_id) WHERE cve_id IS NOT NULL;
CREATE INDEX idx_findings_open ON findings (tenant_id, severity, last_seen DESC)
    WHERE status = 'open';
```

Alternative considered: two tables (`vulnerabilities` + `compliance_findings`),
as the earlier roadmap draft proposed. Rejected because:

- Query patterns the UI actually runs ("all findings on this asset,"
  "everything critical open this week") need a UNION across both tables
  every time.
- The two shapes are 90% identical; the 10% difference (whether `cve_id` is
  populated, whether `remediation` text is common) is cheap to model as
  nullable columns on one table.
- Adding a fourth source later (cloud misconfiguration, SAST import, etc.)
  is one row in an enum, not a new table and a new handler.

The `source_kind` enum matches the three categories the product team
articulated: network vulnerability (recon-driven CVEs), network compliance
(passive/unauthenticated checks — weak TLS, default banners, etc. — not yet
implemented, but slot reserved), and bundle compliance (authenticated deep
scans).

**Partitioning:** monthly by `first_seen`, matching `asset_events` and
`notification_deliveries`. Retention is a Phase 7+ concern; 12 months is
the default.

**`discovered_assets.cves` becomes a denormalized cache.** Phase 4 ingest
continues to write the cves array for list-query performance, but
`findings` is the source of truth. Phase 7 drops the cache column when
queries are migrated.

### D2. Ingest write-through

Discovery scan (nuclei hit):

```go
// Phase 4 — inside the asset_discovered handler, per nuclei hit:
store.InsertFinding(ctx, Finding{
    AssetEndpointID: endpointID,
    ScanID:          &scanID,
    SourceKind:      "network_vuln",
    Source:          "nuclei",
    SourceID:        hit.TemplateID,
    CVEID:           firstOrNil(hit.CVEs),
    Severity:        hit.Severity,
    Title:           hit.TemplateID,
    Status:          "open",
    Evidence:        hit.Evidence,
})
```

Compliance scan (bundle result):

```go
// Phase 4 — inside CreateScanResults (keep existing scan_results write for now):
for _, r := range results {
    store.InsertFinding(ctx, Finding{
        AssetEndpointID: endpointID,
        ScanID:          &scanID,
        SourceKind:      "bundle_compliance",
        Source:          bundleID,
        SourceID:        r.ControlID,
        Severity:        r.Severity,
        Title:           r.Title,
        Status:          statusFromCheck(r.Status),  // pass → resolved, fail → open
        Evidence:        r.Evidence,
        Remediation:     r.Remediation,
    })
}
```

Both paths upsert on `(asset_endpoint_id, source_kind, source, source_id)` so
re-running a scan updates `last_seen` and re-opens status if the finding is
still present, rather than producing duplicate rows.

### D3. `scan_definitions` as the scan configuration entity

```sql
CREATE TABLE scan_definitions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,              -- 'compliance' | 'discovery'
    bundle_id UUID REFERENCES bundles(id),    -- null only for discovery

    -- scope: exactly one of the three must be set
    scope_kind TEXT NOT NULL,        -- 'asset_endpoint' | 'collection' | 'cidr'
    asset_endpoint_id UUID REFERENCES asset_endpoints(id) ON DELETE CASCADE,
    collection_id UUID REFERENCES collections(id) ON DELETE CASCADE,
    cidr CIDR,
    CONSTRAINT scope_exactly_one CHECK (
        (scope_kind = 'asset_endpoint' AND asset_endpoint_id IS NOT NULL
            AND collection_id IS NULL AND cidr IS NULL) OR
        (scope_kind = 'collection' AND collection_id IS NOT NULL
            AND asset_endpoint_id IS NULL AND cidr IS NULL) OR
        (scope_kind = 'cidr' AND cidr IS NOT NULL
            AND asset_endpoint_id IS NULL AND collection_id IS NULL)
    ),

    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    schedule TEXT,                   -- cron expression; NULL = manual-only
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMPTZ,         -- set by scheduler; NULL when disabled or manual
    last_run_at TIMESTAMPTZ,
    last_run_status TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    UNIQUE (tenant_id, name)
);

CREATE INDEX idx_scan_defs_due ON scan_definitions (next_run_at)
    WHERE enabled = TRUE AND schedule IS NOT NULL AND next_run_at IS NOT NULL;
```

The `scope_kind` CHECK is the cleanest way to enforce "exactly one scope"
without adding a discriminator-per-row table. Rewrites in a future
migration are easier than unrolling a lookup table today.

**`scans` gains `scan_definition_id UUID NULL REFERENCES scan_definitions(id)`.**
Nullable because the existing `POST /api/v1/scans` path creates a scan
without a prior definition (true ad-hoc, for manual debugging). All
scheduled and fan-out runs reference a definition.

**One-shot scans collapse into scan_definitions** with `schedule = NULL`
and `scope_kind = 'collection'`. The existing `one_shot_scans` table + the
`parent_one_shot_id` + `inline_predicate` columns on `scans` become
redundant. Phase 7 cleans them up; Phase 5 leaves them read-only alongside
the new path.

### D4. In-process scheduler

A single goroutine in the DC API process polls for due definitions every 30
seconds:

```go
// api/internal/scheduler/scheduler.go
func (s *Scheduler) tick(ctx context.Context) error {
    rows, err := s.db.Query(ctx, `
        UPDATE scan_definitions
        SET next_run_at = compute_next(schedule, NOW())
        WHERE id IN (
            SELECT id FROM scan_definitions
            WHERE enabled = TRUE
              AND schedule IS NOT NULL
              AND next_run_at IS NOT NULL
              AND next_run_at <= NOW()
            FOR UPDATE SKIP LOCKED
            LIMIT 32
        )
        RETURNING id, tenant_id, kind, bundle_id, scope_kind,
                  asset_endpoint_id, collection_id, cidr, agent_id
    `)
    // dispatch each row through the existing scan-creation path
}
```

**Why in-process:** we already have Postgres; `SELECT … FOR UPDATE SKIP
LOCKED` gives HA safety if we scale the API horizontally. A separate
Cloud Run service would need a minimum instance always awake (the polling
loop never sleeps-to-zero), paying money for no benefit at current scale.
This matches the "boring tech, minimal deps" principle.

**Cron parsing:** hand-rolled on the subset we care about (standard 5-field
cron). If parsing grows complex we can pull in `robfig/cron/v3` — a single
small dep. Not today.

**Tick interval:** 30 seconds. Gives us sub-minute precision for
`*/1 * * * *` definitions (a tick will fire within 30s of the minute
boundary, within noise for a scan). Shorter ticks buy nothing at our
workload shape.

**HA safety:** `FOR UPDATE SKIP LOCKED` means two API instances polling
concurrently cannot dispatch the same definition twice. The `next_run_at`
update advances the clock inside the same transaction as the selection, so
a crash mid-dispatch re-schedules the definition on next tick rather than
skipping it.

**Dispatch idempotence:** the scheduler calls the same scan-creation code
path that a manual trigger uses. A scan is created first, then its
directive is published; if the directive publish fails we delete the scan
row and re-queue `next_run_at`. Matches the ingest-idempotence pattern
from ADR 003.

### D5. `POST /api/v1/scan-definitions/{id}/execute` for manual runs

Manual-run button on the UI (Scans → Definitions tab) POSTs to the execute
endpoint. Handler creates a `scans` row with `scan_definition_id = {id}`
and dispatches, bypassing `next_run_at`. `last_run_at` updates but
`next_run_at` is untouched — a manual run does not shift the next
scheduled run.

### D6. Existing `POST /api/v1/scans` stays

The one-off debug path (create a scan against a target without a stored
definition) continues to work. Those scans land with
`scan_definition_id = NULL`. Phase 7 can decide whether to remove the path;
for now it's useful for testing and for the "Run a bundle right now"
workflow where authoring a definition would be overkill.

### D7. URL surface

```
GET    /api/v1/findings                 list; filters: source_kind, source, severity,
                                        status, asset_endpoint_id, collection_id,
                                        cve_id, since, until
GET    /api/v1/findings/{id}            detail
POST   /api/v1/findings/{id}/suppress   status → suppressed
POST   /api/v1/findings/{id}/reopen     status → open

GET    /api/v1/scan-definitions
POST   /api/v1/scan-definitions
GET    /api/v1/scan-definitions/{id}
PUT    /api/v1/scan-definitions/{id}
DELETE /api/v1/scan-definitions/{id}
POST   /api/v1/scan-definitions/{id}/execute    manual run
POST   /api/v1/scan-definitions/{id}/enable
POST   /api/v1/scan-definitions/{id}/disable
```

Existing `/api/v1/asset-sets/*`, `/api/v1/one-shot-scans/*` paths remain
redirects during the 30-day window from ADR 006 D5. Phase 7 removes them.

### D8. UI surfaces land in step with the schema

Phase 4 ships:
- Findings top-level nav item (two tabs: Vulnerabilities, Compliance).
- Asset detail drawer gets the Findings subsection under each expanded
  endpoint.

Phase 5 ships:
- Scans top-level nav gains the three tabs (Definitions / Activity /
  Targets) per `docs/plans/ui-shape.md`.
- The old Scans page's "New scan" button is replaced by
  "New scan definition" on the Definitions tab.
- One-shot page is deleted.

## Consequences

**Positive:**

- One findings table with one shape. Filter, chart, and export logic is
  written once.
- Scheduled scans become a first-class feature without adding a new
  service or dep. Operators don't need external cron.
- The three-way scope enum (`asset_endpoint | collection | cidr`) gives a
  clean mental model for "what does this scan run against?" and maps
  directly to the UI shape.
- Deleting the one-shot code path removes the JSONB `inline_predicate`
  on `scans` — one fewer bespoke schema escape hatch.

**Negative:**

- The findings table is the highest-write table in the system. Phase 4
  should ship with load tests before turning on.
- `SELECT … FOR UPDATE SKIP LOCKED` + polling is subtle; the scheduler
  needs careful unit tests around: tick racing, mid-tick crash recovery,
  clock drift between API instances.
- The CHECK constraint on `scan_definitions.scope_kind` makes adding a
  fourth scope type (`aws_account`? `kubernetes_namespace`?) require a
  migration, not just an enum bump. Acceptable — we'll be adding a new
  scope type rarely enough that a migration is fine.

**Scope boundary:**

- No credential consolidation here (Phase 6 — extends ADR 004).
- No retention worker for `findings`. Drop-partition-older-than-12-months
  is a future PR; for now partitions accumulate.
- No UI widgets for findings on the Dashboard beyond a basic count by
  severity. Richer widget types are follow-on work.

## Open questions

- **OQ1.** Do we need a `findings.assignee_id` column now for "this
  finding is being worked on by user X" workflows? Lean **no** — we don't
  have an issue-tracker story, and adding the column without the workflow
  is clutter.
- **OQ2.** Should `scan_definitions.schedule` accept seconds-resolution cron
  (6-field form) ever? Our scheduler ticks at 30s so the answer is "not
  meaningfully." Stick with 5-field; document.
- **OQ3.** For collection-scoped scan definitions, the set of endpoints can
  grow between ticks. Do we snapshot the membership at dispatch (and thus
  need to re-resolve each tick, which is expensive for big collections) or
  resolve once and store? Lean **re-resolve each tick** — it's what the
  user expects ("run my bundle against every matching endpoint *now*"), and
  we can cache the predicate evaluation cheaply.
- **OQ4.** Scheduler leadership: today a single API instance works fine.
  When we scale horizontally, `FOR UPDATE SKIP LOCKED` lets every instance
  tick safely but means N instances all wake every 30s. Acceptable at N=2;
  worth a `scheduler_leader` advisory lock if we ever run N>4.
