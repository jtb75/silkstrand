-- ADR 006 + ADR 007: asset-first refactor (P1 schema + store layer).
-- Drops the recon-pipeline tables that were built against the old
-- target/scan/one-shot mental model and recreates them against the
-- asset / asset_endpoint / collection / finding / scan_definition model.
--
-- Greenfield strategy (docs/plans/asset-first-execution.md): we preserve
-- tenants, users, agents, bundles, install_tokens, credential_sources,
-- and the (soon-to-be-narrowed) targets table. Everything that touches
-- discovery, rules, notifications, one-shots, scans, or scan_results
-- drops and rebuilds. Data loss here is by design.

-- ============================================================
-- Step 1 — Drop the old recon pipeline + scan_results + scans.
-- Must run BEFORE the UUID cleanup because scans.bundle_id is an
-- FK to bundles without ON UPDATE CASCADE; we can't rewrite bundle
-- ids while those references are still live.
-- Order matters here too: FKs cascade from child to parent.
-- ============================================================
DROP TABLE IF EXISTS scan_results CASCADE;
DROP TABLE IF EXISTS scans CASCADE;
DROP TABLE IF EXISTS one_shot_scans CASCADE;
DROP TABLE IF EXISTS notification_deliveries CASCADE;
DROP TABLE IF EXISTS notification_channels CASCADE;
DROP TABLE IF EXISTS correlation_rules CASCADE;
DROP TABLE IF EXISTS asset_sets CASCADE;
DROP TABLE IF EXISTS asset_events CASCADE;
DROP TABLE IF EXISTS discovered_assets CASCADE;
DROP TABLE IF EXISTS agent_allowlists CASCADE;

-- Drop any leftover columns on surviving tables that referenced the
-- dropped ones. targets.asset_id referenced discovered_assets.
ALTER TABLE targets DROP COLUMN IF EXISTS asset_id;

-- ============================================================
-- Step 2 — UUID randomness cleanup (ADR 006 D10).
-- Rewrite legitimately-leaked dev-seed bundle ids to random v4.
-- Runs AFTER the scans/one_shot_scans/etc. drops in step 1 so no
-- live FK references block the UPDATE. The discovery bundle
-- (11111111-…) is reserved and explicitly excluded.
-- ============================================================
UPDATE bundles
   SET id = uuid_generate_v4()
 WHERE id::text LIKE '00000000-0000-0000-0000-0000000000%'
   AND id <> '11111111-1111-1111-1111-111111111111';

-- ============================================================
-- Step 3 — UUID randomness guard (ADR 006 D10).
-- Refuse to migrate if any non-reserved row still carries a
-- dev-seed-pattern id. This must stay green on every future
-- migration; treat a failure here as a signal to audit the
-- seed path that produced the leak.
-- ============================================================
DO $$
DECLARE leaked INT;
BEGIN
  SELECT COUNT(*) INTO leaked FROM bundles
    WHERE id::text LIKE '00000000-0000-0000-0000-0000000000%'
      AND id <> '11111111-1111-1111-1111-111111111111';
  IF leaked > 0 THEN
    RAISE EXCEPTION 'UUID randomness invariant violated: % bundle rows with dev-seed ids after cleanup.', leaked;
  END IF;
END $$;

-- ============================================================
-- Step 4 — Narrow targets to CIDR / network_range only (ADR 006 D8).
-- ============================================================
DELETE FROM targets WHERE type NOT IN ('cidr', 'network_range');
ALTER TABLE targets DROP CONSTRAINT IF EXISTS targets_target_type_check;
ALTER TABLE targets ADD CONSTRAINT targets_target_type_check
    CHECK (type IN ('cidr', 'network_range'));

-- ============================================================
-- Step 5 — Extend credential_sources.type (ADR 006 roadmap P6).
-- No existing CHECK; add one that allows the pluggable set so the
-- future channels + vault rows can live on the same surface.
-- ============================================================
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'credential_sources_type_check'
  ) THEN
    ALTER TABLE credential_sources ADD CONSTRAINT credential_sources_type_check
      CHECK (type IN (
        'static', 'slack', 'webhook', 'email', 'pagerduty',
        'aws_secrets_manager', 'hashicorp_vault', 'cyberark'
      ));
  END IF;
END $$;

-- ============================================================
-- Step 6 — assets (ADR 006 D2).
-- ============================================================
CREATE TABLE assets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    primary_ip INET,
    hostname TEXT,
    fingerprint JSONB NOT NULL DEFAULT '{}'::jsonb,
    resource_type TEXT NOT NULL DEFAULT 'host',
    source TEXT NOT NULL,
    environment TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_assets_tenant_ip ON assets(tenant_id, primary_ip)
    WHERE primary_ip IS NOT NULL;
CREATE INDEX idx_assets_tenant_last_seen ON assets(tenant_id, last_seen DESC);
CREATE INDEX idx_assets_tenant_source ON assets(tenant_id, source);

-- ============================================================
-- Step 7 — asset_endpoints (ADR 006 D2).
-- ============================================================
CREATE TABLE asset_endpoints (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    port INT NOT NULL,
    protocol TEXT NOT NULL DEFAULT 'tcp',
    service TEXT,
    version TEXT,
    technologies JSONB NOT NULL DEFAULT '[]'::jsonb,
    compliance_status TEXT,
    allowlist_status TEXT,
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

-- ============================================================
-- Step 8 — asset_discovery_sources (ADR 006 D9).
-- ============================================================
CREATE TABLE asset_discovery_sources (
    asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    scan_id UUID,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (asset_id, discovered_at)
);
CREATE INDEX idx_asset_discovery_sources_target ON asset_discovery_sources(target_id);

-- ============================================================
-- Step 9 — asset_events (ADR 006 D4 — FK now points at asset_endpoints).
-- Partitioned monthly by occurred_at, same pattern as migration 011.
-- ============================================================
CREATE TABLE asset_events (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    asset_id UUID NOT NULL,                 -- FK logical → asset_endpoints(id); not enforced because partitioned parent cannot carry FK
    scan_id UUID,
    event_type TEXT NOT NULL,
    severity TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_events_tenant_time ON asset_events(tenant_id, occurred_at DESC);
CREATE INDEX idx_events_asset_time ON asset_events(asset_id, occurred_at DESC);
CREATE INDEX idx_events_type_time ON asset_events(tenant_id, event_type, occurred_at DESC);

CREATE TABLE asset_events_2026_04 PARTITION OF asset_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE asset_events_2026_05 PARTITION OF asset_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE asset_events_2026_06 PARTITION OF asset_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE asset_events_2026_07 PARTITION OF asset_events
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

-- ============================================================
-- Step 10 — collections (ADR 006 D5).
-- ============================================================
CREATE TABLE collections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    scope TEXT NOT NULL DEFAULT 'endpoint',
    predicate JSONB NOT NULL,
    is_dashboard_widget BOOLEAN NOT NULL DEFAULT FALSE,
    widget_kind TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    UNIQUE (tenant_id, name),
    CHECK (scope IN ('asset', 'endpoint', 'finding'))
);
CREATE INDEX idx_collections_tenant ON collections(tenant_id);
CREATE INDEX idx_collections_dashboard ON collections(tenant_id)
    WHERE is_dashboard_widget = TRUE;

-- ============================================================
-- Step 11 — correlation_rules (ADR 006 D6: body → {collection_id}).
-- ============================================================
CREATE TABLE correlation_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    trigger TEXT NOT NULL,
    event_type_filter TEXT,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by TEXT,
    UNIQUE (tenant_id, name, version)
);
CREATE INDEX idx_rules_tenant_trigger_enabled
    ON correlation_rules(tenant_id, trigger, enabled);

-- ============================================================
-- Step 12 — notification_deliveries (points at credential_sources,
-- not a separate channel table). Partitioned monthly.
-- ============================================================
CREATE TABLE notification_deliveries (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    channel_source_id UUID NOT NULL,        -- FK logical → credential_sources(id)
    rule_id UUID,
    event_id UUID,
    severity TEXT,
    status TEXT NOT NULL,
    attempt INT NOT NULL DEFAULT 1,
    response_code INT,
    error TEXT,
    payload JSONB,
    dispatched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, dispatched_at)
) PARTITION BY RANGE (dispatched_at);

CREATE INDEX idx_deliveries_tenant_time ON notification_deliveries(tenant_id, dispatched_at DESC);
CREATE INDEX idx_deliveries_channel_time ON notification_deliveries(channel_source_id, dispatched_at DESC);
CREATE INDEX idx_deliveries_status_time ON notification_deliveries(status, dispatched_at DESC)
    WHERE status IN ('pending','retrying','failed');

CREATE TABLE notification_deliveries_2026_04 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE notification_deliveries_2026_05 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE notification_deliveries_2026_06 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE notification_deliveries_2026_07 PARTITION OF notification_deliveries
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

-- ============================================================
-- Step 13 — scan_definitions (ADR 007 D3).
-- Must exist before scans so scans.scan_definition_id can FK here.
-- ============================================================
CREATE TABLE scan_definitions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    bundle_id UUID REFERENCES bundles(id),
    scope_kind TEXT NOT NULL,
    asset_endpoint_id UUID REFERENCES asset_endpoints(id) ON DELETE CASCADE,
    collection_id UUID REFERENCES collections(id) ON DELETE CASCADE,
    cidr CIDR,
    CONSTRAINT scan_definitions_scope_exactly_one CHECK (
        (scope_kind = 'asset_endpoint' AND asset_endpoint_id IS NOT NULL
            AND collection_id IS NULL AND cidr IS NULL) OR
        (scope_kind = 'collection' AND collection_id IS NOT NULL
            AND asset_endpoint_id IS NULL AND cidr IS NULL) OR
        (scope_kind = 'cidr' AND cidr IS NOT NULL
            AND asset_endpoint_id IS NULL AND collection_id IS NULL)
    ),
    CONSTRAINT scan_definitions_kind_check CHECK (kind IN ('compliance', 'discovery')),
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    schedule TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_run_status TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID,
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_scan_defs_due ON scan_definitions(next_run_at)
    WHERE enabled = TRUE AND schedule IS NOT NULL AND next_run_at IS NOT NULL;
CREATE INDEX idx_scan_defs_tenant ON scan_definitions(tenant_id);

-- ============================================================
-- Step 14 — scans (ADR 007 D3 — points at scan_definitions, nullable).
-- ============================================================
CREATE TABLE scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    scan_definition_id UUID REFERENCES scan_definitions(id) ON DELETE SET NULL,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    asset_endpoint_id UUID REFERENCES asset_endpoints(id) ON DELETE SET NULL,
    bundle_id UUID REFERENCES bundles(id),
    scan_type TEXT NOT NULL DEFAULT 'compliance',
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_scans_tenant_created ON scans(tenant_id, created_at DESC);
CREATE INDEX idx_scans_definition ON scans(scan_definition_id)
    WHERE scan_definition_id IS NOT NULL;
CREATE INDEX idx_scans_tenant_type_status ON scans(tenant_id, scan_type, status);

-- ============================================================
-- Step 15 — findings (ADR 007 D1). Partitioned monthly by first_seen.
-- ============================================================
CREATE TABLE findings (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    asset_endpoint_id UUID NOT NULL,        -- FK logical → asset_endpoints(id)
    scan_id UUID,
    source_kind TEXT NOT NULL,
    source TEXT NOT NULL,
    source_id TEXT,
    cve_id TEXT,
    severity TEXT,
    title TEXT NOT NULL,
    status TEXT NOT NULL,
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

CREATE TABLE findings_2026_04 PARTITION OF findings
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE findings_2026_05 PARTITION OF findings
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE findings_2026_06 PARTITION OF findings
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE findings_2026_07 PARTITION OF findings
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

-- ============================================================
-- Step 16 — credential_mappings (ADR 006 roadmap P6 placeholder).
-- Maps a collection to a credential_source so a scan-definition
-- running against a collection can resolve creds per endpoint.
-- ============================================================
CREATE TABLE credential_mappings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    credential_source_id UUID NOT NULL REFERENCES credential_sources(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (collection_id, credential_source_id)
);
CREATE INDEX idx_credential_mappings_tenant ON credential_mappings(tenant_id);
CREATE INDEX idx_credential_mappings_collection ON credential_mappings(collection_id);

-- ============================================================
-- Step 17 — asset_relationships (placeholder for containers / graph).
-- Empty until R2+. Minimal shape so future migrations can extend.
-- ============================================================
CREATE TABLE asset_relationships (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    parent_asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    child_asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    relationship_type TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (parent_asset_id, child_asset_id, relationship_type)
);
CREATE INDEX idx_asset_relationships_parent ON asset_relationships(parent_asset_id);
CREATE INDEX idx_asset_relationships_child ON asset_relationships(child_asset_id);
