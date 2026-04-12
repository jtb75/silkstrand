-- Run against local dev database (port 15432)
-- The UUID will be auto-generated; use the returned ID when creating scans
INSERT INTO bundles (name, version, framework, target_type)
VALUES ('cis-postgresql-16', '1.0.0', 'python', 'database')
RETURNING id;
