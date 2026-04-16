-- Partial unique index on (tenant_id, type, identifier) for CIDR / network_range
-- targets. Lets the scheduler's CIDR-scope dispatch path safely upsert a
-- target row per (tenant, cidr) pair without racing. The partial clause
-- keeps future per-engine target types (if they return) unconstrained.
CREATE UNIQUE INDEX IF NOT EXISTS targets_cidr_key
    ON targets (tenant_id, type, identifier)
    WHERE type IN ('cidr', 'network_range');
