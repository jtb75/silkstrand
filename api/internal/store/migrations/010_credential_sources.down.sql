DROP INDEX IF EXISTS idx_targets_credential_source;
ALTER TABLE targets DROP COLUMN IF EXISTS credential_source_id;
DROP TABLE IF EXISTS credential_sources;
