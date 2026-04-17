-- Extend bundles table with fields from ADR 010 D11.
ALTER TABLE bundles ADD COLUMN IF NOT EXISTS engine TEXT;
ALTER TABLE bundles ADD COLUMN IF NOT EXISTS control_count INT NOT NULL DEFAULT 0;

-- Note: gcs_path already exists on bundles from the initial schema.

-- Control metadata table per ADR 010 D11.
CREATE TABLE IF NOT EXISTS bundle_controls (
  bundle_id UUID NOT NULL REFERENCES bundles(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,
  name TEXT NOT NULL,
  severity TEXT,
  section TEXT,
  engine TEXT NOT NULL,
  engine_versions JSONB NOT NULL DEFAULT '[]'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (bundle_id, control_id)
);

CREATE INDEX IF NOT EXISTS idx_bundle_controls_engine ON bundle_controls(engine);
CREATE INDEX IF NOT EXISTS idx_bundle_controls_control ON bundle_controls(control_id);
