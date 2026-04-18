-- ADR 005: Audit events surface. Monthly-partitioned, tenant-scoped.
CREATE TABLE audit_events (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT,
    resource_type TEXT,
    resource_id TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant_id, occurred_at, id)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_audit_tenant_type ON audit_events(tenant_id, event_type, occurred_at DESC);
CREATE INDEX idx_audit_resource ON audit_events(tenant_id, resource_id, occurred_at DESC) WHERE resource_id IS NOT NULL;
CREATE INDEX idx_audit_actor ON audit_events(tenant_id, actor_id, occurred_at DESC) WHERE actor_id IS NOT NULL;

-- Seed partitions covering current quarter + a month ahead.
CREATE TABLE audit_events_2026_04 PARTITION OF audit_events FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE audit_events_2026_05 PARTITION OF audit_events FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE audit_events_2026_06 PARTITION OF audit_events FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE audit_events_2026_07 PARTITION OF audit_events FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
