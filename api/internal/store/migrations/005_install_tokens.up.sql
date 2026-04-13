-- Single-use, short-lived bootstrap tokens that let install.sh register
-- an agent without the admin copying a long-lived API key.
CREATE TABLE install_tokens (
    token_hash BYTEA PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    created_by TEXT,                  -- user_id from the JWT at creation time (for audit)
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    used_agent_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_install_tokens_tenant ON install_tokens(tenant_id, created_at DESC);
