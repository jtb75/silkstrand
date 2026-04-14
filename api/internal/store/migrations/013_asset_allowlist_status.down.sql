DROP INDEX IF EXISTS idx_assets_tenant_allowlist;
ALTER TABLE discovered_assets
    DROP COLUMN IF EXISTS allowlist_checked_at,
    DROP COLUMN IF EXISTS allowlist_status;
