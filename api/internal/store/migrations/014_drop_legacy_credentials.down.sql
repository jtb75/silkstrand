-- Recreate the legacy credentials table (schema only — data is gone).
-- This mirrors the shape after migrations 001, 004, and 008.
CREATE TABLE credentials (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    target_id UUID NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    encrypted_data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credentials_target ON credentials(target_id);
CREATE UNIQUE INDEX credentials_target_unique ON credentials(target_id);
