# R0 — Recon pipeline data model (ADR 003)

## 1. Summary

R0 lands the full schema backbone for ADR 003 (discovery, events, rule engine, notifications, asset sets, one-shot scans) in a single migration `011_recon_pipeline.up.sql`, plus a backfill that gives every existing target an owning `discovered_assets` row. After R0 the database is shaped for R1a through R1c; only application code changes between those phases.

## 2. Schema

All SQL lives in one file. Inline comments explain column intent.

```sql
-- 011_recon_pipeline.up.sql

-- ============================================================
-- discovered_assets: current-state inventory (D4, D6)
-- One row per (tenant, ip, port). Manual target creation and
-- discovery both upsert here; whichever path lands first owns
-- `source`, subsequent passes enrich in place.
-- ============================================================
CREATE TABLE discovered_assets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ip INET NOT NULL,
    port INT NOT NULL,                       -- 0 for host-only (v1.1 cloud assets)
    hostname TEXT,
    service TEXT,                            -- 'http','postgresql','rdp',...
    version TEXT,
    technologies JSONB NOT NULL DEFAULT '[]',-- httpx fingerprints, tags
    cves JSONB NOT NULL DEFAULT '[]',        -- [{id,severity,template,found_at}]
    compliance_status TEXT,                  -- null|'candidate'|'targeted'|'pass'|'fail'
    source TEXT NOT NULL,                    -- 'manual' | 'discovered'
    environment TEXT,                        -- mirrors targets.environment for filtering
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scan_id UUID,                       -- FK added soft; scans table already exists
    missed_scan_count INT NOT NULL DEFAULT 0,-- D4 gone-detection (see §6)
    metadata JSONB NOT NULL DEFAULT '{}',    -- escape hatch
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, ip, port)
);
CREATE INDEX idx_assets_tenant_last_seen  ON discovered_assets(tenant_id, last_seen DESC);
CREATE INDEX idx_assets_tenant_first_seen ON discovered_assets(tenant_id, first_seen DESC);
CREATE INDEX idx_assets_tenant_service    ON discovered_assets(tenant_id, service);
CREATE INDEX idx_assets_tenant_env        ON discovered_assets(tenant_id, environment);
CREATE INDEX idx_assets_tenant_compliance ON discovered_assets(tenant_id, compliance_status);
-- jsonb_path_ops is tighter for containment queries (cves @> '[{"id":"..."}]')
CREATE INDEX idx_assets_cves_gin          ON discovered_assets USING GIN (cves jsonb_path_ops);
CREATE INDEX idx_assets_tech_gin          ON discovered_assets USING GIN (technologies jsonb_path_ops);
-- Functional index for "has CVEs" / "CVE count > 0" filter chip.
CREATE INDEX idx_assets_has_cves ON discovered_assets(tenant_id)
    WHERE jsonb_array_length(cves) > 0;

-- ============================================================
-- asset_events: append-only change log (D4).
-- Partitioned by month on occurred_at (see §4).
-- ============================================================
CREATE TABLE asset_events (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,                 -- no FK: partitioned parent; enforced via asset_id
    asset_id UUID NOT NULL,                  -- intentionally no FK (cross-partition FKs awkward)
    scan_id UUID,
    event_type TEXT NOT NULL,                -- new_asset|asset_gone|new_cve|cve_resolved|
                                             --  version_changed|port_opened|port_closed|
                                             --  compliance_pass|compliance_fail
    severity TEXT,                           -- info|low|medium|high|critical
    payload JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_events_tenant_time   ON asset_events(tenant_id, occurred_at DESC);
CREATE INDEX idx_events_asset_time    ON asset_events(asset_id, occurred_at DESC);
CREATE INDEX idx_events_type_time     ON asset_events(tenant_id, event_type, occurred_at DESC);

-- Seed partitions: previous, current, next 2 months.
CREATE TABLE asset_events_2026_03 PARTITION OF asset_events
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE asset_events_2026_04 PARTITION OF asset_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE asset_events_2026_05 PARTITION OF asset_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE asset_events_2026_06 PARTITION OF asset_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

-- ============================================================
-- asset_sets: saved predicates over discovered_assets (D13).
-- ============================================================
CREATE TABLE asset_sets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    predicate JSONB NOT NULL,                -- shape defined in §5
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_asset_sets_tenant ON asset_sets(tenant_id);

-- ============================================================
-- correlation_rules: versioned match→action rules (D2).
-- `version` increments on update; the row with highest version
-- for a (tenant_id, name) is the live one. Older rows retained
-- for audit.
-- ============================================================
CREATE TABLE correlation_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    trigger TEXT NOT NULL,                   -- 'asset_discovered' | 'asset_event'
    event_type_filter TEXT,                  -- for asset_event, optional event_type
    body JSONB NOT NULL,                     -- {match:{...}, actions:[{type,...}]}
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX idx_rules_tenant_trigger_enabled
    ON correlation_rules(tenant_id, trigger, enabled);

-- ============================================================
-- notification_channels (D12).
-- `config` is JSONB; secrets inside (webhook `secret`,
-- pagerduty `routing_key`, slack `webhook_url`) are stored as
-- base64'd AES-256-GCM ciphertext, same pattern credential_sources
-- uses for static credentials. Decryption happens in app layer.
-- ============================================================
CREATE TABLE notification_channels (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,                      -- 'webhook'|'slack'|'email'|'pagerduty'
    config JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_channels_tenant ON notification_channels(tenant_id);

-- ============================================================
-- notification_deliveries (D12). Append-only audit; partitioned
-- like asset_events.
-- ============================================================
CREATE TABLE notification_deliveries (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    channel_id UUID NOT NULL,                -- soft FK
    rule_id UUID,                            -- soft FK; nullable for manual tests
    event_id UUID,
    severity TEXT,
    status TEXT NOT NULL,                    -- 'pending'|'sent'|'failed'|'retrying'
    attempt INT NOT NULL DEFAULT 1,
    response_code INT,
    error TEXT,
    payload JSONB,                           -- final rendered body (post-template)
    dispatched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, dispatched_at)
) PARTITION BY RANGE (dispatched_at);

CREATE INDEX idx_deliveries_tenant_time   ON notification_deliveries(tenant_id, dispatched_at DESC);
CREATE INDEX idx_deliveries_channel_time  ON notification_deliveries(channel_id, dispatched_at DESC);
CREATE INDEX idx_deliveries_status_time   ON notification_deliveries(status, dispatched_at DESC)
    WHERE status IN ('pending','retrying','failed');

CREATE TABLE notification_deliveries_2026_03 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE notification_deliveries_2026_04 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE notification_deliveries_2026_05 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE notification_deliveries_2026_06 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

-- ============================================================
-- one_shot_scans: parent record for fan-out scans (D13).
-- child rows are regular `scans` tagged via parent_one_shot_id.
-- ============================================================
CREATE TABLE one_shot_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bundle_id UUID NOT NULL REFERENCES bundles(id),
    asset_set_id UUID REFERENCES asset_sets(id) ON DELETE SET NULL,
    inline_predicate JSONB,                  -- set when asset_set_id IS NULL
    max_concurrency INT NOT NULL DEFAULT 10,
    rate_limit_pps INT,
    total_targets INT,                       -- filled at fan-out time
    completed_targets INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending|running|completed|failed
    triggered_by TEXT,                       -- 'rule:<name>'|'user:<id>'|'api'
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CHECK (asset_set_id IS NOT NULL OR inline_predicate IS NOT NULL)
);
CREATE INDEX idx_oneshot_tenant_created ON one_shot_scans(tenant_id, created_at DESC);
CREATE INDEX idx_oneshot_status         ON one_shot_scans(status)
    WHERE status IN ('pending','running');

-- Link child scans to their parent one-shot (nullable — normal scans unaffected).
ALTER TABLE scans
    ADD COLUMN parent_one_shot_id UUID REFERENCES one_shot_scans(id) ON DELETE SET NULL;
CREATE INDEX idx_scans_parent_one_shot ON scans(parent_one_shot_id)
    WHERE parent_one_shot_id IS NOT NULL;

-- ============================================================
-- targets refactor (D6).
-- Every target now points at a discovered_assets row.
-- ============================================================
ALTER TABLE targets
    ADD COLUMN asset_id UUID REFERENCES discovered_assets(id) ON DELETE SET NULL;
CREATE INDEX idx_targets_asset ON targets(asset_id);

-- ---------- Backfill -----------------------------------------
-- Parse targets.identifier into (ip, port). v1 targets are
-- network_range shapes; for non-IP identifiers (hostname, CIDR,
-- range) we store the identifier's first resolvable IP, or a
-- sentinel 0.0.0.0/port=0 row when we can't parse — enough to
-- preserve the invariant `targets.asset_id IS NOT NULL` without
-- blocking the migration. Application layer cleans up sentinels
-- on the next discovery pass.

INSERT INTO discovered_assets (
    id, tenant_id, ip, port, source, first_seen, last_seen, environment, created_at, updated_at
)
SELECT
    uuid_generate_v4(),
    t.tenant_id,
    COALESCE(
        -- bare IPv4
        (SELECT (regexp_match(t.identifier, '^(\d+\.\d+\.\d+\.\d+)$'))[1]::INET),
        -- CIDR: take network address
        (SELECT host(network(t.identifier::cidr))::INET
         WHERE t.identifier ~ '^\d+\.\d+\.\d+\.\d+/\d+$'),
        '0.0.0.0'::INET
    ),
    COALESCE((t.config->>'port')::INT, 0),
    'manual',
    t.created_at,
    t.updated_at,
    t.environment,
    t.created_at,
    t.updated_at
FROM targets t
ON CONFLICT (tenant_id, ip, port) DO NOTHING;

-- Wire targets to assets.
UPDATE targets t
   SET asset_id = a.id
  FROM discovered_assets a
 WHERE a.tenant_id = t.tenant_id
   AND a.ip = COALESCE(
       (SELECT (regexp_match(t.identifier, '^(\d+\.\d+\.\d+\.\d+)$'))[1]::INET),
       (SELECT host(network(t.identifier::cidr))::INET
        WHERE t.identifier ~ '^\d+\.\d+\.\d+\.\d+/\d+$'),
       '0.0.0.0'::INET)
   AND a.port = COALESCE((t.config->>'port')::INT, 0)
   AND a.source = 'manual';
```

Down migration drops in reverse dependency order, drops the `targets.asset_id`/`scans.parent_one_shot_id` columns, leaves `targets` intact.

## 3. Migration plan

**Single file, `011_recon_pipeline.up.sql`.** Rationale:
- All 7 tables + 2 ALTERs are interdependent (targets→assets, one_shot→asset_sets, scans→one_shot, deliveries→channels).
- golang-migrate runs each file in its own transaction; splitting forces partial deploys where a half-built schema is live.
- Rollback is "drop the whole thing" — simpler with one file.
- Size (~250 lines SQL) is fine; `001_initial.up.sql` established the one-big-file precedent.

**Idempotency.** Not required — golang-migrate gates on `schema_migrations.version`. But the backfill `INSERT ... ON CONFLICT DO NOTHING` makes re-running the data portion safe if someone ever manually re-executes.

**Ordering within the file.** Tables with no FKs first (`discovered_assets`, partitioned parents), then dependents (`one_shot_scans` refs `asset_sets` + `bundles`), then ALTERs, then backfill.

**Rollback (`011_recon_pipeline.down.sql`).**
```sql
ALTER TABLE scans DROP COLUMN IF EXISTS parent_one_shot_id;
ALTER TABLE targets DROP COLUMN IF EXISTS asset_id;
DROP TABLE IF EXISTS one_shot_scans;
DROP TABLE IF EXISTS notification_deliveries;   -- cascades to partitions
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS correlation_rules;
DROP TABLE IF EXISTS asset_sets;
DROP TABLE IF EXISTS asset_events;              -- cascades to partitions
DROP TABLE IF EXISTS discovered_assets;
```
No attempt to re-hydrate a pre-backfill targets state — rollback is a dev/staging operation.

## 4. Partitioning

**Decision: partition both `asset_events` and `notification_deliveries` by month from day 1.**

Sizing (napkin math):
- 100 tenants × 1k assets × daily discovery = 100k asset upserts/day. Steady-state events (no drift) ≈ `last_seen` touches which we don't log. Churn events ≈ 1% → ~1k events/day → 30k/mo at this tier. That's small.
- Bad day (CVE feed refresh, version roll): every asset emits `new_cve` or `version_changed` → 100k events in a few hours.
- Worst case within 12 months (500 tenants × 5k assets × weekly churn): ~1M events/week.
- `notification_deliveries`: roughly 10–50% of events per tenant that have rules wired.

Unpartitioned we still perform at ~10M rows, but:
- GIN/BTREE rebuilds on the 3-year trim (D4) are painful without partition drop.
- `DELETE FROM asset_events WHERE occurred_at < NOW() - INTERVAL '3 years'` on 100M+ rows = hours of bloat.
- Partition drop is instant + reclaims disk.

**Setup** (shown in §2): `PARTITION BY RANGE(occurred_at)`, monthly partitions, primary key includes the partition column.

**Partition maintenance.** Use `pg_partman` if already in the stack; otherwise a 20-line Go cron hook in the API runs daily at 00:05 UTC:

```go
// creates partitions for current+1, current+2, current+3 months; idempotent.
CREATE TABLE IF NOT EXISTS asset_events_YYYY_MM PARTITION OF asset_events
    FOR VALUES FROM ('YYYY-MM-01') TO (...);
```

Retention (3 years for `asset_events`, 1 year default for `deliveries`) handled by the same cron: `DROP TABLE asset_events_YYYY_MM` for partitions older than cutoff. Leave the concrete cron scaffolding to R1c — R0 only ships the partitioning.

## 5. Predicate JSONB shape

One grammar, used in `correlation_rules.body.match` and `asset_sets.predicate`. Minimal, extensible.

**Schema (informal):**
```
predicate := term | compound
term      := { "<field>": <value> | <operator_obj> }
operator_obj := { "$eq": v } | { "$in": [v,...] } | { "$cidr": "10.0.0.0/8" }
              | { "$regex": "..." } | { "$gt": n } | { "$lt": n } | { "$exists": bool }
compound  := { "$and": [predicate, ...] }
            | { "$or":  [predicate, ...] }
            | { "$not":  predicate }

Fields (v1): ip, port, hostname, service, version, environment,
            source, compliance_status, technologies.<tag>, cves.<id>,
            first_seen, last_seen
```

Bare scalar = `$eq`. Bare array in a field = `$in`. CIDR is a first-class operator because IP membership is the single most common rule shape.

**Examples.**

1. Shadow-IT RDP on the wrong segment (from ADR D2):
```json
{ "$and": [
  { "service": "rdp" },
  { "ip": { "$cidr": "10.50.0.0/16" } },
  { "$not": { "ip": { "$cidr": "10.50.10.0/24" } } }
] }
```

2. "Compliance candidates" asset set (Postgres 16.x in prod):
```json
{ "$and": [
  { "service": "postgresql" },
  { "version": { "$regex": "^16\\." } },
  { "environment": "production" }
] }
```

3. "Assets with critical CVEs seen in the last 7 days":
```json
{ "$and": [
  { "cves": { "$exists": true } },
  { "cves.severity": { "$in": ["critical","high"] } },
  { "last_seen": { "$gt": "2026-04-06T00:00:00Z" } }
] }
```

Evaluation (R1a+) compiles to parameterized SQL; CIDR → `ip <<= $N::cidr`, regex → `~`, `$in` → `= ANY($N)`. JSONB field accessors (`cves.severity`) lower to `cves @> '[{"severity":"critical"}]'` (containment, GIN-indexed).

## 6. `asset_gone` recommendation

**Recommendation: "N consecutive missed scans" using `missed_scan_count`, default N=3, tenant-overridable.**

Mechanics:
- Each discovery scan computes a `scan_session_id` = `scans.id`.
- For every asset in scope of the scan (same target/CIDR coverage), if it wasn't observed, `missed_scan_count += 1`. If observed, reset to 0 and bump `last_seen`.
- When `missed_scan_count` transitions from `N-1` → `N`, write an `asset_gone` event. Asset row stays (we want history + `first_seen`).
- A later reappearance resets the counter and emits `asset_reappeared` (add to `event_type` enum — cheap).

**Why not "M days stale":** Discovery cadence varies (daily vs weekly vs ad-hoc one-shot). A 14-day stale threshold is wrong for a weekly cadence and too eager for a monthly one. Counting scans is cadence-independent and matches operator intuition ("three in a row, it's gone").

**Tenant override:** store `asset_gone_threshold INT DEFAULT 3` on `tenants` in a follow-up (not R0 — add when someone asks). R0 schema already captures the counter; R1a implements the logic.

**Caveat to flag for R1a:** "in scope of the scan" needs care. A tenant running one discovery scan against `10.0.0.0/24` and another against `10.1.0.0/24` must not mark 10.1 assets as missed by the 10.0 scan. Track scope at the scan level; only increment counters for assets whose `(ip, port)` fell inside that scan's target CIDRs.

## 7. Open items left for R1a

- **Scan-scope bookkeeping** for `missed_scan_count` (previous bullet). Needs either a `scans.discovery_scope` JSONB column or computing coverage from the target at eval time.
- **Sentinel assets from backfill.** Targets whose identifier is a hostname or IP range map to `0.0.0.0:0` sentinel rows; R1a's first discovery pass should reconcile these into real rows and update `targets.asset_id`.
- **Rule body validation.** Schema allows any JSONB; R1b needs a JSON-schema validator plus a pre-save dry-run against a sample asset.
- **Predicate→SQL compiler** specifics (identifier allowlist, injection safety, GIN usage audit).
- **Notification template language.** `payload` column captures the rendered output; the templating engine choice (Go `text/template` vs a mini-DSL) is deferred to R1c.
- **Partition-maintenance owner.** Cron in the API process vs a separate Go binary vs pg_partman. R1c deadline.
- **Retention policy config surface.** Per-tenant override for `asset_events` (3y default) and `deliveries` (1y default) — schema allows it (just a tenant column), UI deferred.
- **`scans.target_id` nullability** when spawned from one-shot (a one-shot targets an asset, not a legacy target row). Likely needs `scans.target_id` to become nullable or a synthetic target row per asset — defer the call to R1c when the dispatcher lands.
