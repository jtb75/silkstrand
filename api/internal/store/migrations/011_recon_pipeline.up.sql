-- ADR 003 R0: full schema for the recon pipeline and supporting features.
-- Lands all R0/R1a/R1b/R1c tables in one atomic migration. Application
-- code (handlers, store methods) follows in subsequent PRs.

-- ============================================================
-- discovered_assets: current-state inventory (D4, D6).
-- One row per (tenant, ip, port). Manual target creation and
-- discovery both upsert here.
-- ============================================================
CREATE TABLE discovered_assets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ip INET NOT NULL,
    port INT NOT NULL,
    hostname TEXT,
    service TEXT,
    version TEXT,
    technologies JSONB NOT NULL DEFAULT '[]',
    cves JSONB NOT NULL DEFAULT '[]',
    compliance_status TEXT,
    source TEXT NOT NULL,
    environment TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scan_id UUID,
    missed_scan_count INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, ip, port)
);
CREATE INDEX idx_assets_tenant_last_seen ON discovered_assets(tenant_id, last_seen DESC);
CREATE INDEX idx_assets_tenant_first_seen ON discovered_assets(tenant_id, first_seen DESC);
CREATE INDEX idx_assets_tenant_service ON discovered_assets(tenant_id, service);
CREATE INDEX idx_assets_tenant_env ON discovered_assets(tenant_id, environment);
CREATE INDEX idx_assets_tenant_compliance ON discovered_assets(tenant_id, compliance_status);
CREATE INDEX idx_assets_cves_gin ON discovered_assets USING GIN (cves jsonb_path_ops);
CREATE INDEX idx_assets_tech_gin ON discovered_assets USING GIN (technologies jsonb_path_ops);
CREATE INDEX idx_assets_has_cves ON discovered_assets(tenant_id) WHERE jsonb_array_length(cves) > 0;

-- ============================================================
-- asset_events: append-only change log (D4). Partitioned monthly.
-- ============================================================
CREATE TABLE asset_events (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    asset_id UUID NOT NULL,
    scan_id UUID,
    event_type TEXT NOT NULL,
    severity TEXT,
    payload JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_events_tenant_time ON asset_events(tenant_id, occurred_at DESC);
CREATE INDEX idx_events_asset_time ON asset_events(asset_id, occurred_at DESC);
CREATE INDEX idx_events_type_time ON asset_events(tenant_id, event_type, occurred_at DESC);

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
    predicate JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_asset_sets_tenant ON asset_sets(tenant_id);

-- ============================================================
-- correlation_rules: versioned match→action rules (D2).
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
-- notification_channels (D12). Webhook secrets stored as base64
-- AES-256-GCM ciphertext inside config JSONB (same pattern
-- credential_sources uses for static credentials).
-- ============================================================
CREATE TABLE notification_channels (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);
CREATE INDEX idx_channels_tenant ON notification_channels(tenant_id);

-- ============================================================
-- notification_deliveries (D12). Append-only audit; partitioned monthly.
-- ============================================================
CREATE TABLE notification_deliveries (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    channel_id UUID NOT NULL,
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
CREATE INDEX idx_deliveries_channel_time ON notification_deliveries(channel_id, dispatched_at DESC);
CREATE INDEX idx_deliveries_status_time ON notification_deliveries(status, dispatched_at DESC)
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
-- ============================================================
CREATE TABLE one_shot_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bundle_id UUID NOT NULL REFERENCES bundles(id),
    asset_set_id UUID REFERENCES asset_sets(id) ON DELETE SET NULL,
    inline_predicate JSONB,
    max_concurrency INT NOT NULL DEFAULT 10,
    rate_limit_pps INT,
    total_targets INT,
    completed_targets INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    triggered_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CHECK (asset_set_id IS NOT NULL OR inline_predicate IS NOT NULL)
);
CREATE INDEX idx_oneshot_tenant_created ON one_shot_scans(tenant_id, created_at DESC);
CREATE INDEX idx_oneshot_status ON one_shot_scans(status)
    WHERE status IN ('pending','running');

-- ============================================================
-- scans extensions:
--   parent_one_shot_id : link child rows to a one-shot fan-out parent
--   scan_type          : 'compliance' | 'discovery' (default for back-compat)
--   discovery_scope    : JSONB list of CIDRs/IPs scanned, used by R1b
--                        asset_gone reaper to know coverage
--   target_id NULLABLE : one-shot scans address an asset, not a target row
-- ============================================================
ALTER TABLE scans
    ADD COLUMN parent_one_shot_id UUID REFERENCES one_shot_scans(id) ON DELETE SET NULL,
    ADD COLUMN scan_type TEXT NOT NULL DEFAULT 'compliance',
    ADD COLUMN discovery_scope JSONB,
    ALTER COLUMN target_id DROP NOT NULL;
CREATE INDEX idx_scans_parent_one_shot ON scans(parent_one_shot_id)
    WHERE parent_one_shot_id IS NOT NULL;
CREATE INDEX idx_scans_tenant_type_status ON scans(tenant_id, scan_type, status);

-- ============================================================
-- targets refactor (D6): every target points at a discovered_assets
-- row. Backfill below populates one asset per existing target.
-- ============================================================
ALTER TABLE targets
    ADD COLUMN asset_id UUID REFERENCES discovered_assets(id) ON DELETE SET NULL;
CREATE INDEX idx_targets_asset ON targets(asset_id);

-- ---------- Backfill -----------------------------------------
-- Parse targets.identifier into (ip, port). For non-IP identifiers
-- (hostname, range), assets land at sentinel (0.0.0.0, 0); the first
-- discovery pass reconciles them. ON CONFLICT DO NOTHING keeps it
-- safe to re-run by hand.
INSERT INTO discovered_assets (
    id, tenant_id, ip, port, source, first_seen, last_seen, environment, created_at, updated_at
)
SELECT
    uuid_generate_v4(),
    t.tenant_id,
    COALESCE(
        (SELECT (regexp_match(t.identifier, '^(\d+\.\d+\.\d+\.\d+)$'))[1]::INET),
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
