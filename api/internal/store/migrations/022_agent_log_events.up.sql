-- Persist agent log events so the console can load history on open.
-- Partitioned by occurred_at for easy retention cleanup.
CREATE TABLE agent_log_events (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    scan_id TEXT,
    level TEXT NOT NULL,
    msg TEXT NOT NULL,
    attrs JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE INDEX idx_agent_logs_agent_time ON agent_log_events (agent_id, occurred_at DESC);
CREATE INDEX idx_agent_logs_tenant_time ON agent_log_events (tenant_id, occurred_at DESC);
CREATE INDEX idx_agent_logs_scan ON agent_log_events (scan_id, occurred_at DESC) WHERE scan_id IS NOT NULL;

-- Current + next two months partitions
CREATE TABLE agent_log_events_2026_04 PARTITION OF agent_log_events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE agent_log_events_2026_05 PARTITION OF agent_log_events
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE agent_log_events_2026_06 PARTITION OF agent_log_events
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
