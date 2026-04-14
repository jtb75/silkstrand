-- ADR 003 D11 follow-up: persist each agent's most recently reported scan
-- allowlist snapshot so the server can tag discovered_assets with a
-- display status (allowlisted / out_of_policy / unknown). Policy is still
-- owned and enforced by the agent; this is purely informational.
CREATE TABLE agent_allowlists (
    agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    snapshot_hash TEXT NOT NULL,
    allow JSONB NOT NULL DEFAULT '[]',
    deny JSONB NOT NULL DEFAULT '[]',
    rate_limit_pps INT NOT NULL DEFAULT 0,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
