-- ADR 004 Phase C0: plumbing only. Introduces credential_sources +
-- targets.credential_source_id FK. Backfills one `static` source per
-- existing credentials row. The legacy `credentials` table is left in
-- place (readable and writable) through the rollback window; a later
-- migration drops it.

CREATE TABLE credential_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    config JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_credential_sources_tenant ON credential_sources(tenant_id);

ALTER TABLE targets
    ADD COLUMN credential_source_id UUID REFERENCES credential_sources(id) ON DELETE SET NULL;
CREATE INDEX idx_targets_credential_source ON targets(credential_source_id);

-- Backfill: reuse credentials.id as credential_sources.id so the
-- mapping is deterministic and the UPDATE below is a simple join.
INSERT INTO credential_sources (id, tenant_id, type, config, created_at, updated_at)
SELECT
    c.id,
    c.tenant_id,
    'static',
    jsonb_build_object(
        'type', c.type,
        'encrypted_data', encode(c.encrypted_data, 'base64')
    ),
    c.created_at,
    c.created_at
FROM credentials c;

UPDATE targets t
   SET credential_source_id = c.id
  FROM credentials c
 WHERE c.target_id = t.id;
