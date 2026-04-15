-- Seed data for the DC (data-center) database.
-- Idempotent: safe to run multiple times.
-- Targets the local Docker Postgres on port 15432.
--
-- POST-MIGRATION-017: targets are narrowed to CIDR / network_range per
-- ADR 006 D8. Legacy engine-specific seed targets are gone — P2 will
-- reintroduce a seed asset_endpoint + credential_source path for the
-- compliance demo flow. Today the seed only covers tenant + agent +
-- CIDR target + the discovery bundle reference.

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

-- Target (CIDR — the only supported tenant-facing target shape after
-- migration 017). Random UUID, not a dev-seed pattern one, so the
-- randomness guard in 017 doesn't flag future migrations.
INSERT INTO targets (id, tenant_id, agent_id, type, identifier, config, environment) VALUES (
  uuid_generate_v4(),
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000010',
  'cidr',
  '192.168.0.0/24',
  '{}',
  'dev'
) ON CONFLICT DO NOTHING;
