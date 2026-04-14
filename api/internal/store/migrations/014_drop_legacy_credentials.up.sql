-- ADR 004 C0 close-out: the dual-write rollback seam has baked in and
-- every credential lives in credential_sources (type='static'). Drop
-- the legacy credentials table.
DROP TABLE IF EXISTS credentials;
