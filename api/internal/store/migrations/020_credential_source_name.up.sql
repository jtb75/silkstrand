--- Add a human-friendly name column to credential_sources so users can
--- label their DB/host credentials (e.g. "studio-mssql-sa").
ALTER TABLE credential_sources ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
