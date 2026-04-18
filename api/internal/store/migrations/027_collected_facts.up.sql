-- ADR 011 PR 1: collected_facts table for storing raw facts from collectors.
-- Partitioned by collected_at for efficient retention cleanup.

CREATE TABLE collected_facts (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    asset_endpoint_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    collector_id TEXT NOT NULL,
    facts JSONB NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, collected_at, id)
) PARTITION BY RANGE (collected_at);

CREATE INDEX idx_facts_endpoint ON collected_facts(asset_endpoint_id, collected_at DESC);
CREATE INDEX idx_facts_scan ON collected_facts(scan_id);
CREATE INDEX idx_facts_collector ON collected_facts(collector_id, collected_at DESC);

CREATE TABLE collected_facts_2026_04 PARTITION OF collected_facts FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE collected_facts_2026_05 PARTITION OF collected_facts FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE collected_facts_2026_06 PARTITION OF collected_facts FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE collected_facts_2026_07 PARTITION OF collected_facts FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
