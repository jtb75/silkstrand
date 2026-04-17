-- Extend credential_mappings to support three scope kinds:
--   collection  — existing behavior (bind a source to a collection)
--   asset_endpoint — bind directly to a single endpoint
--   asset       — bind to an asset (covers all its endpoints)

ALTER TABLE credential_mappings
  ADD COLUMN scope_kind TEXT NOT NULL DEFAULT 'collection',
  ADD COLUMN asset_endpoint_id UUID REFERENCES asset_endpoints(id) ON DELETE CASCADE,
  ADD COLUMN asset_id UUID REFERENCES assets(id) ON DELETE CASCADE;

-- Drop the old unique constraint (collection_id + credential_source_id)
-- and replace with a broader composite unique index.
ALTER TABLE credential_mappings DROP CONSTRAINT IF EXISTS credential_mappings_collection_id_credential_source_id_key;

CREATE UNIQUE INDEX credential_mappings_scope_key ON credential_mappings (
  credential_source_id,
  COALESCE(collection_id::text, ''),
  COALESCE(asset_endpoint_id::text, ''),
  COALESCE(asset_id::text, '')
);

-- Exactly one scope target must be set per row.
ALTER TABLE credential_mappings ADD CONSTRAINT credential_mappings_scope_check CHECK (
  (scope_kind = 'collection' AND collection_id IS NOT NULL AND asset_endpoint_id IS NULL AND asset_id IS NULL) OR
  (scope_kind = 'asset_endpoint' AND asset_endpoint_id IS NOT NULL AND collection_id IS NULL AND asset_id IS NULL) OR
  (scope_kind = 'asset' AND asset_id IS NOT NULL AND collection_id IS NULL AND asset_endpoint_id IS NULL)
);

-- Make collection_id nullable (was NOT NULL) so endpoint/asset scopes work.
ALTER TABLE credential_mappings ALTER COLUMN collection_id DROP NOT NULL;

-- Indexes for the new FK columns.
CREATE INDEX idx_credential_mappings_endpoint ON credential_mappings(asset_endpoint_id)
  WHERE asset_endpoint_id IS NOT NULL;
CREATE INDEX idx_credential_mappings_asset ON credential_mappings(asset_id)
  WHERE asset_id IS NOT NULL;
