-- ADR 003 D11 follow-up: tag each discovered_asset with its status
-- relative to the owning agent's reported scan allowlist.
--   'unknown'       — no snapshot from the agent yet, or no owning agent
--   'allowlisted'   — IP (and hostname if any) covered by allow rules
--   'out_of_policy' — explicitly denied, or not allowed
-- Informational only; used by the UI to gate the Promote button.
ALTER TABLE discovered_assets
    ADD COLUMN allowlist_status TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN allowlist_checked_at TIMESTAMPTZ;

CREATE INDEX idx_assets_tenant_allowlist ON discovered_assets(tenant_id, allowlist_status);
