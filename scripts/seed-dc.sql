-- Seed data for the DC (data-center) database.
-- Idempotent: safe to run multiple times.
-- Targets the local Docker Postgres on port 15432.

-- Tenant
INSERT INTO tenants (id, name, status, config) VALUES (
  '00000000-0000-0000-0000-000000000001',
  'Test Tenant',
  'active',
  '{}'
) ON CONFLICT (id) DO NOTHING;

-- Agent (key_hash = SHA-256 of 'test-agent-key')
-- Verify: echo -n "test-agent-key" | shasum -a 256
INSERT INTO agents (id, tenant_id, name, key_hash) VALUES (
  '00000000-0000-0000-0000-000000000010',
  '00000000-0000-0000-0000-000000000001',
  'local-test-agent',
  '4d25b7920b389a66f0c2f265b145519aa821b431e96eaa7d2927b8e9ef275cdb'
) ON CONFLICT (id) DO NOTHING;

-- Target (points at local Docker Postgres)
INSERT INTO targets (id, tenant_id, agent_id, type, identifier, config, environment) VALUES (
  '00000000-0000-0000-0000-000000000020',
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000010',
  'database',
  'localhost:15432/silkstrand',
  '{"host": "localhost", "port": 15432, "database": "silkstrand", "username": "silkstrand", "password": "localdev", "sslmode": "disable"}',
  'dev'
) ON CONFLICT (id) DO NOTHING;

-- Bundle (CIS PostgreSQL 16)
INSERT INTO bundles (id, name, version, framework, target_type) VALUES (
  '00000000-0000-0000-0000-000000000030',
  'cis-postgresql-16', '1.0.0', 'python', 'database'
) ON CONFLICT (id) DO NOTHING;
