DROP INDEX IF EXISTS idx_targets_asset;
ALTER TABLE targets DROP COLUMN IF EXISTS asset_id;

DROP INDEX IF EXISTS idx_scans_parent_one_shot;
DROP INDEX IF EXISTS idx_scans_tenant_type_status;
ALTER TABLE scans
    DROP COLUMN IF EXISTS parent_one_shot_id,
    DROP COLUMN IF EXISTS scan_type,
    DROP COLUMN IF EXISTS discovery_scope,
    ALTER COLUMN target_id SET NOT NULL;

DROP TABLE IF EXISTS one_shot_scans;
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS correlation_rules;
DROP TABLE IF EXISTS asset_sets;
DROP TABLE IF EXISTS asset_events;
DROP TABLE IF EXISTS discovered_assets;
