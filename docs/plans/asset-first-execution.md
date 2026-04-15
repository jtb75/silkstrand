# Asset-first refactor — execution plan

Concrete sequence for landing the asset-first refactor defined in ADR 006,
ADR 007, and `docs/plans/ui-shape.md`. This doc supersedes the phased
roadmap in the approved plan file — it's more aggressive, because we've
decided to treat this as **greenfield**: we preserve only data the tenant
actually needs, not data the old code happened to write.

## Context

The design layer is done: two ADRs + one UI spec are on `main`. We have a
live-but-tiny production footprint (one tenant, a handful of agents,
hundreds of discovered assets, no saved correlation rules or notification
channels yet). That's small enough that "migration strategy" is a non-issue
— we can drop and rebuild most of the recon schema in one migration without
losing anything the user cares about.

"Greenfield, save only what we need" means: keep tenants, users,
memberships, agents, bundles, install_tokens, credential_sources. Drop
everything else that was built on the old model (discovered_assets,
asset_events, asset_sets, correlation_rules, notification_channels,
notification_deliveries, one_shot_scans, scan_results, scans) and recreate
it against the new model. If a tenant had a working recon pipeline before,
they'll have a working recon pipeline after — just pointing at the new
tables.

## Greenfield scope

### Keep (migrated as-is or with additive columns)

| Table | Treatment |
|---|---|
| `tenants`, `users`, `memberships`, `invitations`, `password_resets` | No change. |
| `agents`, `install_tokens` | No change. |
| `bundles` | No change. |
| `credential_sources` | Extend `type` enum to add `slack`, `webhook`, `email`, `pagerduty`, `aws_secrets_manager`, `hashicorp_vault`, `cyberark`. Existing `static` rows unchanged. |
| `targets` | Narrow immediately to CIDR / network-range only. Existing non-CIDR rows are dropped (tenant only has the one CIDR). |

### Drop and recreate under the new model

| Old table | Replacement | Data preserved? |
|---|---|---|
| `discovered_assets` | `assets` + `asset_endpoints` | **No.** Re-populate on next discovery scan. Tenant has no irreplaceable data here. |
| `asset_events` | `asset_events` (same shape; `asset_id` FK repoints at `asset_endpoints.id`) | No. |
| `asset_sets` | `collections` (extended scope: asset / endpoint / finding) | No. Tenant has none. |
| `correlation_rules` | `correlation_rules` (body.match → `{collection_id}`) | No. Tenant has none. |
| `notification_channels` | `credential_sources` rows with type `slack` / `webhook` / etc. | No. Tenant has none. |
| `notification_deliveries` | Same table, points at `credential_sources.id` instead | No. Partition data lost; ok. |
| `one_shot_scans` | `scan_definitions` rows with `schedule = NULL` and `scope_kind = 'collection'` | No. Tenant has none. |
| `scans`, `scan_results` | `scans` (pointing at `scan_definitions`) + `findings` | No — historical scan_results drop cleanly. |

### New tables (created fresh)

`assets`, `asset_endpoints`, `asset_discovery_sources`, `collections`,
`scan_definitions`, `findings`, `credential_mappings`,
`asset_relationships` (placeholder, empty until containers land).

### Single migration vs phased migrations

One big migration (`017_asset_first.sql`). It:

1. `DROP` the old recon-pipeline tables (and related columns on `scans`).
2. `CREATE` the new set.
3. Tightens `targets.target_type` check.

No backfills, no dual-write, no 30-day URL alias windows. The old
`asset-sets` / `one-shot-scans` handlers are deleted in the same PR that
adds the new ones. Code churn is larger per-PR but the tree stays coherent.

## Phases

Five phases, each shippable independently. P1 and P2 are serial; P3–P5 can
run in parallel. P6 is integration + polish.

### P1 — Schema + store layer (backend)

**Scope:** The migration, new model types, new store functions, compile-passing
delete of everything that referenced dropped tables (handlers included —
they'll be stubs returning 501 until the next phases fill them in). Targets
handler narrows to CIDR.

**Surface after P1:** API compiles, starts, connects to DB, healthchecks green.
Every non-stub endpoint returns 501 Not Implemented except tenant auth,
agents, bundles, install tokens, and the narrowed targets CRUD.

**UUID randomness audit (ADR 006 D10).** The migration runs a guard query
against the existing database before applying schema changes:

```sql
-- Fail the migration if any non-reserved row has a dev-seed pattern id.
DO $$
DECLARE leaked INT;
BEGIN
  SELECT COUNT(*) INTO leaked FROM bundles
    WHERE id::text LIKE '00000000-0000-0000-0000-0000000000%'
      AND id <> '11111111-1111-1111-1111-111111111111';
  IF leaked > 0 THEN
    RAISE EXCEPTION 'UUID randomness invariant violated: % bundle rows with dev-seed ids. Clean before migration.', leaked;
  END IF;
END $$;
```

The same check extends to any other table discovered to have leaked
dev-seed ids. The known prod-leaked rows (today's CIS bundles at
`…000030/031/032`) are cleaned up by a one-liner `UPDATE bundles SET id =
uuid_generate_v4() WHERE …` in the same migration BEFORE the guard runs.
All bundle references that previously pointed at fixed ids land on fresh
random UUIDs.

Reserved ids (the discovery bundle `11111111-…`) are explicitly excluded
from the guard and re-seeded at their fixed value as part of P1.

**One engineer, one PR.**

**Branches unblocked:** P2, then everything else.

### P2 — Ingest + rules engine (backend)

**Scope:** Reimplement the `asset_discovered` WSS handler against
`UpsertAsset` + `UpsertAssetEndpoint`. Wire `EvaluateAsset` into the ingest
path. Reimplement the rule action dispatchers (`suggest_target`,
`auto_create_target`, `notify`, `run_one_shot_scan`) against the new
schema. Manual target creation (CIDR only) updates.

**Surface after P2:** A discovery scan completes end-to-end and populates
assets + asset_endpoints. No findings yet; no rule UI yet.

**One engineer, one PR.**

**Branches unblocked:** P3, P4, P5.

### P3 — Findings + Scans (backend + UI) — **parallel**

**Scope:** Findings table ingest (nuclei hits + bundle results write-through),
findings handlers, scan_definitions CRUD + scheduler, the scans page
reshape (Definitions / Activity / Targets tabs), the new Findings page,
Scan Results page gets the Findings tab.

**Two engineers:**
- **P3-backend** — `findings` + ingest write-through, `scan_definitions` +
  scheduler goroutine, handler layer, `POST /scan-definitions/{id}/execute`.
- **P3-frontend** — Scans page tabs, Findings page, Scan Results findings
  tab.

### P4 — Collections + Assets UI (backend + UI) — **parallel**

**Scope:** Collections handlers (including scope=`finding` runtime dispatch),
collection membership API, assets page reshape (3-tab layout, Bulk Actions
bar, Coverage column), asset detail drawer (6-section), "Save as
Collection" UX.

**Two engineers:**
- **P4-backend** — collections CRUD, scope dispatch in evaluator, coverage
  roll-ups in asset API response.
- **P4-frontend** — Collections page, Assets page (3 tabs + bulk), asset
  detail drawer.

### P5 — Dashboard + Settings / Credentials (UI-heavy) — **parallel**

**Scope:** Dashboard (KPI row + widget grid + Suggested Actions),
Settings-as-tabs (Profile / Team / Credentials / Audit-placeholder),
Credentials consolidation (DB / Integrations / Vaults sub-sections),
bulk credential-to-endpoint mapping UX.

**Two engineers:**
- **P5-a** — Dashboard + Suggested Actions widget computation.
- **P5-b** — Settings reshape + Credentials page + credential_mappings
  CRUD.

### P6 — Integration + polish

**Scope:** End-to-end smoke against a Docker-host lab, rules firing on
discovery, scheduled scan tick, notifications delivered, UI round-trips
clean. Fix whatever cross-workstream seams broke. No new features.

**One engineer, one PR, probably two days.**

## Team structure

Use `TeamCreate` to spin a five-agent team once P2 merges. Each agent gets
its own worktree (isolated so they don't step on each other) and a narrow
remit from the phase list above. Parent session coordinates, reviews each
PR, and resolves cross-workstream decisions.

```
        ┌── team silkstrand-asset-first ──┐
        │                                  │
        │   coordinator  (this session)    │
        │        │                         │
        │        ├── P1: backend-schema    │ serial, merges first
        │        │                         │
        │        ├── P2: backend-ingest    │ serial, merges second
        │        │                         │
        │        ├── P3-backend            │ ─┐
        │        ├── P3-frontend           │  │
        │        ├── P4-backend            │  ├── parallel after P2
        │        ├── P4-frontend           │  │
        │        ├── P5-a                  │  │
        │        ├── P5-b                  │ ─┘
        │        │                         │
        │        └── P6: integration       │ serial after P3/P4/P5
        │                                  │
        └──────────────────────────────────┘
```

Agent briefs:

| Agent | Deliverable | Files it owns |
|---|---|---|
| `backend-schema` (P1) | Migration 017 + model types + empty store funcs + handler stubs. API compiles, healthchecks pass. | `api/internal/store/migrations/017_*.sql`, `api/internal/model/`, `api/internal/store/postgres.go`, `api/internal/handler/*.go` (stub returns) |
| `backend-ingest` (P2) | Discovery ingest against new schema; `EvaluateAsset` wired; action dispatchers rewired; CIDR-only target CRUD. | `api/cmd/silkstrand-api/main.go` (ingest handler), `api/internal/rules/engine.go`, `api/internal/handler/target.go` |
| `p3-backend` | Findings ingest + handlers; `scan_definitions` CRUD + scheduler goroutine; `POST /scan-definitions/{id}/execute`. | `api/internal/handler/findings.go`, `api/internal/handler/scan_definitions.go`, new `api/internal/scheduler/` package, updates to nuclei/bundle ingest. |
| `p3-frontend` | Scans page (3 tabs), Findings page (2 tabs), Scan Results findings tab. | `web/src/pages/ScanDefinitions.tsx`, `ScanActivity.tsx`, `Findings.tsx`, `ScanResults.tsx` (findings tab), `web/src/api/client.ts` additions. |
| `p4-backend` | Collections CRUD (with `scope = finding` runtime dispatch); collection membership API; coverage roll-ups on the asset API response. | `api/internal/handler/collections.go`, `api/internal/rules/predicate.go` (scope dispatch), `api/internal/handler/asset.go` (coverage roll-up). |
| `p4-frontend` | Collections page (My + Widgets tabs, Query Preview), Assets page 3-tab layout with Bulk Actions + Coverage column, asset detail drawer with 6 sections. | `web/src/pages/Collections.tsx`, `web/src/pages/Assets.tsx`, `web/src/components/AssetDetailDrawer.tsx`, `AssetsBulkActions.tsx` (new). |
| `p5-a` | Dashboard — KPI row, widget grid, Suggested Actions server computation + render. | `web/src/pages/Dashboard.tsx`, new `web/src/components/DashboardWidgets/`, `api/internal/handler/dashboard.go` (server-side suggestion engine). |
| `p5-b` | Settings reshape (tabs); Credentials page (DB / Integrations / Vaults sub-sections); credential_mappings CRUD + bulk-apply UX. | `web/src/pages/Settings.tsx`, `web/src/pages/Credentials.tsx`, `api/internal/handler/credential_mappings.go`. |
| `integration` (P6) | E2E on Docker-host lab; fix cross-workstream seams; UI-polish sweep. | Cross-cutting. |

### Coordination rules

- **Branch off `main` for P1 and P2 sequentially.** Each merges before the
  next starts.
- **P3–P5 branch off `main` once P2 is merged.** Six worktrees in parallel.
- **API contract changes require coordinator approval.** If a P3-backend
  agent needs to add a field to the asset API response for P4-frontend,
  coordinator records the change in this file's "API contract log"
  section and both agents consume it.
- **No direct backend/frontend edits across workstream lines.** If
  p4-frontend needs a backend change, it files a change note that p4-backend
  picks up. Coordinator unblocks if either is stuck.
- **Test discipline.** Each agent owns unit tests for its workstream.
  Integration tests land in P6 once the full surface is in.

### API contract log

Any shared API change negotiated between agents lands here as a single line
so every worktree sees it. Format:

```
YYYY-MM-DD  owner  [endpoint]  change
```

*(Empty at plan-draft time. Populated during execution.)*

## Verification

Per phase:

- **P1.** `make dev-deps && make seed && make run`. API comes up. `curl
  /readyz` returns 200. `psql ... \dt` shows new tables, no old recon
  tables. Non-implemented handlers return 501 with a clear body.
- **P2.** Run a discovery scan on studio against 192.168.0.0/24. Assets +
  endpoints rows appear. `asset_events` partitions populate. No errors in
  agent logs or API logs.
- **P3.** Same discovery scan — `findings` rows appear with
  `source='nuclei'`. Create a scan_definition with `schedule='*/5 * * *
  *'`; watch `next_run_at` advance and a `scans` row materialize within 5
  minutes. Scans page shows all three tabs.
- **P4.** Save the current Assets filter as a Collection; reopen it and
  confirm it loads the same set. Multi-select three endpoints and hit
  "Create Scan" — the definition is created pre-populated.
- **P5.** Dashboard renders all four KPI cards with live data. Suggested
  Actions lists real coverage gaps. Settings → Credentials lists existing
  credential_sources with a new Integrations sub-section containing any
  Slack/webhook entries.
- **P6.** One full operator flow end-to-end: discover → findings appear →
  create scheduled compliance-scan definition via the Dashboard suggestion
  → scan runs → findings update → Slack notification fires.

## Sequencing summary

```
P1  ─────────>                                                     ~1 week
     │
P2  ─┴────────>                                                    ~3–5 days
      │
P3-be ┴──────────────────────>                                     ~2 weeks
P3-fe ┴──────────────────────>                                     ~2 weeks
P4-be ┴──────────────────────>                                     ~2 weeks
P4-fe ┴──────────────────────>                                     ~2 weeks
P5-a  ┴──────────────────────>                                     ~1.5 weeks
P5-b  ┴──────────────────────>                                     ~1.5 weeks
                              │
                              └── P6 ─>                            ~2 days
```

Estimated wall-clock from P1 kickoff to P6 merge: **~3 weeks** with the
six-agent parallel phase, vs ~8 weeks serial. Slowest critical path is P3
or P4 (~2 weeks each).

## Risks

- **Scheduler correctness under load.** `SELECT … FOR UPDATE SKIP LOCKED`
  with a 30s tick is simple but the dispatch idempotence (scan row created
  → directive published → crash) has a narrow race window. Integration
  test in P6 must exercise the crash-recovery path.
- **Handler 501 stubs in P1 break the UI on stage.** Mitigation: deploy
  P1 to stage but **don't** tag prod until P2 lands. Stage can live with
  a broken Assets page for a few days; prod cannot.
- **Six parallel worktrees touching `web/src/api/client.ts`.** Conflicts
  are guaranteed. Coordinator merges client.ts additions first (bite-sized
  PR per workstream, merge as they land) to minimize late merge-hell.
- **Agent context drift.** Each agent has a narrow remit but is working on
  a fast-moving codebase. Coordinator's job is to keep each agent's briefing
  fresh by pointing them at `main` head before they start and catching
  API-contract changes before they diverge.

## What happens to production during this

- v0.1.48 stays live. Prod gets tagged v0.1.49+ only after P6 merges.
- Stage reflects `main` continuously. Expect stage to be partially broken
  during P1 (501s) and P3–P5 (incomplete UI). That's the cost of
  parallelism; accept it.
- No production customer traffic; only the studio tenant sees the stage
  deploy.
