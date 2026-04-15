# ADR 006: Asset-first data model

**Status:** Proposed
**Date:** 2026-04-15
**Related:** [ADR 003](./003-recon-pipeline.md) (recon pipeline that produced
today's `discovered_assets`), [ADR 004](./004-credential-resolver.md) (credential
sources — Phase 6 of the roadmap extends these), [ADR 005](./005-audit-events.md)
(audit surface — writes will reference new asset / endpoint ids), roadmap at
`docs/plans/` (see the approved "Asset-First Refactor" plan).

---

## Context

SilkStrand shipped as a compliance tool: the user-facing nouns were **Target →
Scan → Result**. After R0/R1a/R1b/R1c the live data flow drifted — discovery
writes rows into `discovered_assets`, `asset_events` track change, the rule
engine evaluates predicates, one-shot scans fan out over predicates that bypass
targets entirely. The mental model in the UI never caught up, which leaves
"endpoint," "collection," "scheduled scan," and "unified credential" scattered
across ad-hoc surfaces.

This ADR locks in the data model for the first half of the asset-first
refactor — specifically, Phases 1–3 of the roadmap:

- **Phase 1.** Split `discovered_assets` into `assets` (host-level) and
  `asset_endpoints` (port-level). Future-proof for containers and cloud
  resources via `fingerprint` and `resource_type`.
- **Phase 2.** Promote Asset Sets to **Collections** — the unifying concept
  for saved predicates, Assets-page filter state, rule triggers, and dashboard
  widgets.
- **Phase 3.** Rule `match` predicates become references to a collection
  (`match.collection_id`) instead of each rule carrying its own inline JSONB.

ADR 007 (to follow) will cover Phases 4–5: the findings split
(`vulnerabilities` + `compliance_findings`) and the scan-definitions +
scheduler surface. Phase 6 (consolidated credentials) extends ADR 004. Phase 7
is cleanup and does not need its own ADR.

## Problem

Four data-model problems have compounded:

1. **Host vs endpoint conflation.** `discovered_assets` today is one row per
   `(tenant_id, ip, port)`. Fields that are host-level (hostname, environment,
   source) are denormalized across every endpoint row on that host. Fields
   that are endpoint-level (service, version, cves, compliance_status,
   allowlist_status) can't be queried or modeled at the host level. Containers
   and cloud resources don't fit either dimension cleanly.

2. **Three ways to express "which assets do I care about?"** Asset Sets carry
   JSONB predicates, the Assets page carries hardcoded filter chips, and the
   Dashboard has no filter concept at all. The same question asked three
   different ways produces three different answers.

3. **Rule predicates are inline.** Each correlation rule stores its own
   `body.match` JSONB. Changes to an Asset Set you want to track don't
   propagate into rules that conceptually target the "same" asset population.

4. **`EvaluateAsset` is defined but never called.** The rule engine exists —
   `api/internal/rules/engine.go` has `EvaluateAsset(asset, rules) (actions,
   error)` — but nothing in the asset-ingest path invokes it. Rules exist in
   the UI and the DB but never actually fire on discovery. That's a product
   gap we can close alongside the schema work.

## Decisions

### D1. Greenfield migration strategy

New tables land alongside the old. Old tables become read-only (no new writes)
at the same PR that flips reads over. A later cleanup PR drops them. No
dual-write window.

Rationale: dual-write is the source of most schema-migration bugs — the two
tables drift, the reconciliation logic is the new hot path, rollback needs a
second migration. For the scale we operate at (a single tenant with 10⁴
endpoints is on the high end), a backfill + cutover fits inside one migration
window and is easy to reason about. Each phase's migration is independently
reversible by restoring from the pre-migration backup.

### D2. `assets` and `asset_endpoints`

```sql
CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    primary_ip INET,                -- nullable: cloud resources may not have one
    hostname TEXT,
    fingerprint JSONB NOT NULL DEFAULT '{}'::jsonb,
    resource_type TEXT NOT NULL DEFAULT 'host',
                                    -- 'host' | 'container' | 'cloud_resource' | ...
    source TEXT NOT NULL,           -- 'discovered' | 'manual'
    environment TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, primary_ip)  -- partial, WHERE primary_ip IS NOT NULL
);

CREATE TABLE asset_endpoints (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    port INT NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    service TEXT,
    version TEXT,
    technologies JSONB NOT NULL DEFAULT '[]'::jsonb,
    compliance_status TEXT,
    allowlist_status TEXT,          -- 'allowlisted' | 'out_of_policy' | 'unknown'
    allowlist_checked_at TIMESTAMPTZ,
    last_scan_id UUID,
    missed_scan_count INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (asset_id, port, protocol)
);

CREATE INDEX idx_asset_endpoints_asset ON asset_endpoints(asset_id);
CREATE INDEX idx_asset_endpoints_service ON asset_endpoints(service)
    WHERE service IS NOT NULL;
CREATE INDEX idx_asset_endpoints_last_seen ON asset_endpoints(last_seen DESC);
```

**Column split rationale.** Every column from `discovered_assets` moves
deterministically to one side. Host-level: `tenant_id, primary_ip, hostname,
source, environment, first_seen, created_at` (also the two new columns
`fingerprint` and `resource_type`). Endpoint-level: `port, service, version,
technologies, cves, compliance_status, allowlist_status, allowlist_checked_at,
last_seen, last_scan_id, missed_scan_count, metadata`.

Note: `cves` (JSONB array) is intentionally **not** in `asset_endpoints`.
Phase 4 (ADR 007) introduces `vulnerabilities` as a real table; keeping
endpoint-level cve state denormalized here would just migrate a design bug.
During Phases 1–3 the cve list is read from `discovered_assets` (read-only).

### D3. `fingerprint` is JSONB, not TEXT

JSONB so a single column can hold multiple fingerprint kinds:

```json
{
  "tls_cert_sha256": "…",
  "http_body_hash":  "…",
  "banner_fnv32":    "…",
  "container_image_digest": "sha256:…",
  "cloud_resource_id":      "arn:aws:…"
}
```

Alternative considered: a single TEXT column holding one canonical fingerprint.
Rejected because we already know we need at least three kinds (TLS, HTTP,
container-image) and the shape varies by resource_type. A JSONB column costs
nothing today and avoids a migration the first time we add a second
fingerprint.

### D4. `asset_events.asset_id` points at `asset_endpoints.id`

Event types today (`new_asset`, `version_changed`, `new_cve`, `cve_resolved`,
`port_opened`) are all port-scoped. The table's `asset_id` column already
references an endpoint-identity row in `discovered_assets`. After the split,
the same FK points at `asset_endpoints.id`. No semantic change, no payload
changes.

Alternative considered: repoint at `assets.id` and add an `endpoint_id` column.
Rejected because the events' payload and meaning are port-scoped; adding a
second FK would require rewriting every event type's emitter and every
consumer. The current shape is already "correct" under the new names.

### D5. `collections` replaces `asset_sets`

```sql
CREATE TABLE collections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    scope TEXT NOT NULL DEFAULT 'endpoint',   -- 'asset' | 'endpoint'
    predicate JSONB NOT NULL,
    is_dashboard_widget BOOLEAN NOT NULL DEFAULT FALSE,
    widget_kind TEXT,                         -- 'count' | 'table' | …  (nullable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    UNIQUE (tenant_id, name)
);
```

Migration is a rename + two added columns. Existing Asset Sets become
collections with `scope='endpoint'` (today's predicates all target rows from
`discovered_assets`, which map to endpoints post-split).

The predicate evaluator (`api/internal/rules/predicate.go:21`) is already
shared between Asset Sets and Rules. No duplication; no new evaluator.

**URL handling.** `/api/v1/asset-sets/*` returns HTTP 301 to the corresponding
`/api/v1/collections/*` route for 30 days. After that window the alias
returns 404 and the route is removed (cleanup in Phase 7).

### D6. Rules reference collections by id

Rule `body` becomes:

```json
{
  "match": { "collection_id": "<uuid>" },
  "actions": [ … ]
}
```

Migration rewrites every existing rule's inline `body.match` JSONB into an
auto-created collection (name: `rule: <rule-name>`, predicate: the old match
verbatim; upsert on a hash of the predicate so N rules sharing a predicate
produce one collection). The rule's `body.match` then collapses to
`{collection_id}`.

`EvaluateAsset` dereferences the collection at rule-load time and applies the
stored predicate via the existing matcher. One level of indirection; no new
evaluator.

### D7. `EvaluateAsset` gets wired into ingest

In the same PR as D6 we add a call site in `api/cmd/silkstrand-api/main.go`'s
`asset_discovered` handler:

```go
// After UpsertAsset + UpsertAssetEndpoint + AppendAssetEvents:
actions, err := ruleEngine.EvaluateAsset(ctx, assetView, events)
// Dispatch each action via the existing action handlers
//   (suggest_target, auto_create_target, notify, run_one_shot_scan).
```

The action dispatchers already exist (they're called today only from the
one-shot scan path and the manual promotion path). This closes a standing
product gap: rules-in-the-UI and rules-in-the-DB have existed since R1b
without ever firing on a real discovery event.

### D8. Targets stay, but narrow

Out of scope for Phase 1–3 code, but the data-model intent documented here:
`targets` is not removed. Post-refactor, new scans reference targets only when
`scope_kind='cidr'` (Phase 5 — ADR 007). Compliance scans against a discovered
endpoint bypass targets entirely. Existing rows are retained; Phase 7 cleanup
tightens the `target_type` check to CIDR / network-range only and flags any
non-conforming rows.

### D9. Discovery provenance as a join table

`asset_discovery_sources` records how each asset first showed up in the
inventory:

```sql
CREATE TABLE asset_discovery_sources (
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    scan_id UUID,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (asset_id, discovered_at)
);
```

Populated in the same ingest path as `UpsertAsset`. One row per discovery
event (so an asset rediscovered by a different CIDR target later produces a
second row, and we can see the full provenance).

Rationale: the UI shape plan (`docs/plans/ui-shape.md` § Asset detail view)
wants a "how did this asset get on my list?" Lifecycle section. A column on
`assets` would only capture the *first* provenance and would conflate the
identity-stable bits of an asset with its discovery metadata. A dedicated
join table keeps `assets` narrow and lets us answer richer questions later
("show me everything the `192.168.0.0/24` target has ever found").

`target_id` is nullable because manual-source assets have no target, and
nullable-with-ON-DELETE-SET-NULL because dropping a target shouldn't cascade
into asset history.

## Consequences

**Positive:**

- One obvious place for each field (host vs endpoint). Containers and cloud
  resources fit the `assets` row without conflation.
- Collections become the single unifying filter concept — saved queries, rule
  triggers, dashboard widgets, and Assets-page filter state all reference the
  same `collections` row. "Ask the same question once, get the same answer
  everywhere."
- Rules finally fire on discovery (D7 closes the gap).
- `fingerprint` + `resource_type` don't cost us today and save a migration
  when R2 (AWS cloud discovery) lands.

**Negative:**

- Schema churn: `asset_events.asset_id` FK repoints; two handler call sites
  rewire (`api/cmd/silkstrand-api/main.go:463`, `api/internal/handler/target.go:86`).
  Contained, but real.
- `GET /api/v1/assets` must flatten `assets ⋈ asset_endpoints` in the handler
  for UI compatibility during Phases 1–6. One extra join, documented and not
  load-bearing.
- The rule-body migration (D6) is irreversible without a DB restore — once
  inline predicates become collection_id references, re-inlining the predicate
  would require joining on a soft-deleted collection's stored predicate.
  Acceptable because the old shape offers no tenant value to preserve.

**Scope boundary:**

- No code changes to findings, scheduler, credentials, or Settings UI in this
  ADR. Those arrive in ADR 007 (Phase 4–5) and Phase 6.
- No Dashboard widget framework beyond the minimal `is_dashboard_widget`
  boolean + `widget_kind` enum column on `collections`. A real widget layer
  is a Phase 2 UI effort, not schema.

## Open questions

- **OQ1.** Does the `scope` column on `collections` need to be enforced at
  predicate-evaluation time (i.e., refuse to match a `scope='endpoint'`
  collection against an `asset` row)? Lean yes — the evaluator should fail
  closed rather than silently pass nonsense predicates.
- **OQ2.** `assets.primary_ip` is nullable to accommodate cloud resources.
  During Phase 1 that's hypothetical (no cloud-resource ingestion exists
  until R2). Do we want a CHECK constraint ensuring `primary_ip IS NOT NULL`
  when `resource_type = 'host'`, or defer validation to the handler? Lean
  handler — CHECK constraints interact poorly with future cloud-ingest code.
- **OQ3.** The rule-body migration in D6 needs an upsert key for
  auto-created collections. Using `SHA256(predicate_canonicalized)` as the
  collection name works but produces ugly names. Alternative: name them
  `rule: <rule-name>` and accept that two rules with identical predicates
  get two collections. Lean the latter — ugly names create worse UX than
  duplicate rows.
