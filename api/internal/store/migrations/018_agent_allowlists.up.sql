-- ADR 003 D11 (restored after the P1 greenfield migration 017):
-- re-introduce the per-agent scan-allowlist snapshot table. The agent
-- remains the sole policy authority; this row exists so the API can
-- stamp asset_endpoints.allowlist_status + serve GET /api/v1/agents/{id}/allowlist
-- in the UI.
CREATE TABLE agent_allowlists (
    agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    snapshot_hash TEXT NOT NULL,
    allow JSONB NOT NULL DEFAULT '[]',
    deny JSONB NOT NULL DEFAULT '[]',
    rate_limit_pps INT NOT NULL DEFAULT 0,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
