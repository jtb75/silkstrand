-- Reverse 021: restore credential_mappings to collection-only scope.

-- Remove rows that use the new scopes (they have no collection_id).
DELETE FROM credential_mappings WHERE scope_kind <> 'collection';

-- Drop indexes + constraint + unique index.
DROP INDEX IF EXISTS idx_credential_mappings_asset;
DROP INDEX IF EXISTS idx_credential_mappings_endpoint;
ALTER TABLE credential_mappings DROP CONSTRAINT IF EXISTS credential_mappings_scope_check;
DROP INDEX IF EXISTS credential_mappings_scope_key;

-- Restore NOT NULL on collection_id and the original unique constraint.
ALTER TABLE credential_mappings ALTER COLUMN collection_id SET NOT NULL;
ALTER TABLE credential_mappings ADD CONSTRAINT credential_mappings_collection_id_credential_source_id_key
  UNIQUE (collection_id, credential_source_id);

-- Drop the new columns.
ALTER TABLE credential_mappings DROP COLUMN IF EXISTS asset_id;
ALTER TABLE credential_mappings DROP COLUMN IF EXISTS asset_endpoint_id;
ALTER TABLE credential_mappings DROP COLUMN IF EXISTS scope_kind;
