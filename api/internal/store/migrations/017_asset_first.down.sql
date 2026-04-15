-- Reverse of 017_asset_first.up.sql. Greenfield (see execution plan):
-- we only need to undo the schema, not the data. Restore the old recon
-- tables as empty shells so anything running against 016 still compiles.

DROP TABLE IF EXISTS asset_relationships CASCADE;
DROP TABLE IF EXISTS credential_mappings CASCADE;
DROP TABLE IF EXISTS findings CASCADE;
DROP TABLE IF EXISTS scans CASCADE;
DROP TABLE IF EXISTS scan_definitions CASCADE;
DROP TABLE IF EXISTS notification_deliveries CASCADE;
DROP TABLE IF EXISTS correlation_rules CASCADE;
DROP TABLE IF EXISTS collections CASCADE;
DROP TABLE IF EXISTS asset_events CASCADE;
DROP TABLE IF EXISTS asset_discovery_sources CASCADE;
DROP TABLE IF EXISTS asset_endpoints CASCADE;
DROP TABLE IF EXISTS assets CASCADE;

ALTER TABLE targets DROP CONSTRAINT IF EXISTS targets_target_type_check;
ALTER TABLE credential_sources DROP CONSTRAINT IF EXISTS credential_sources_type_check;

-- Empty shells for the pre-017 recon surface. No data, no indexes beyond
-- primary keys — the down migration is only here so the migrator can
-- reverse cleanly; it is not intended for real rollback.
CREATE TABLE discovered_assets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    ip INET NOT NULL,
    port INT NOT NULL,
    hostname TEXT, service TEXT, version TEXT,
    technologies JSONB NOT NULL DEFAULT '[]',
    cves JSONB NOT NULL DEFAULT '[]',
    compliance_status TEXT, source TEXT NOT NULL, environment TEXT,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_scan_id UUID, missed_scan_count INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}',
    allowlist_status TEXT NOT NULL DEFAULT 'unknown',
    allowlist_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, ip, port)
);

CREATE TABLE asset_events (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL, asset_id UUID NOT NULL, scan_id UUID,
    event_type TEXT NOT NULL, severity TEXT,
    payload JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE TABLE asset_sets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL, description TEXT, predicate JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE correlation_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL, version INT NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    trigger TEXT NOT NULL, event_type_filter TEXT,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), created_by TEXT,
    UNIQUE (tenant_id, name, version)
);

CREATE TABLE notification_channels (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name TEXT NOT NULL, type TEXT NOT NULL, config JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE notification_deliveries (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL, channel_id UUID NOT NULL,
    rule_id UUID, event_id UUID, severity TEXT,
    status TEXT NOT NULL, attempt INT NOT NULL DEFAULT 1,
    response_code INT, error TEXT, payload JSONB,
    dispatched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, dispatched_at)
) PARTITION BY RANGE (dispatched_at);

CREATE TABLE one_shot_scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    bundle_id UUID NOT NULL REFERENCES bundles(id),
    asset_set_id UUID REFERENCES asset_sets(id) ON DELETE SET NULL,
    inline_predicate JSONB,
    max_concurrency INT NOT NULL DEFAULT 10,
    rate_limit_pps INT, total_targets INT,
    completed_targets INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    triggered_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dispatched_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
    CHECK (asset_set_id IS NOT NULL OR inline_predicate IS NOT NULL)
);

CREATE TABLE scans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    target_id UUID REFERENCES targets(id) ON DELETE SET NULL,
    bundle_id UUID REFERENCES bundles(id),
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scan_type TEXT NOT NULL DEFAULT 'compliance',
    parent_one_shot_id UUID REFERENCES one_shot_scans(id) ON DELETE SET NULL,
    discovery_scope JSONB
);

CREATE TABLE scan_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scan_id UUID NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    control_id TEXT NOT NULL, title TEXT NOT NULL,
    status TEXT NOT NULL, severity TEXT,
    evidence JSONB, remediation TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_allowlists (
    agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    snapshot_hash TEXT NOT NULL,
    allow JSONB NOT NULL, deny JSONB NOT NULL,
    rate_limit_pps INT NOT NULL DEFAULT 0,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE targets ADD COLUMN IF NOT EXISTS asset_id UUID REFERENCES discovered_assets(id) ON DELETE SET NULL;
