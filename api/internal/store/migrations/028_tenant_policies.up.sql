CREATE TABLE tenant_policies (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,
  origin TEXT NOT NULL,              -- 'derived' | 'custom'
  based_on TEXT,                     -- original builtin control_id (for derived)
  name TEXT NOT NULL,
  severity TEXT NOT NULL,
  rego_source TEXT NOT NULL,
  collector_id TEXT NOT NULL,
  fact_keys JSONB NOT NULL DEFAULT '[]'::jsonb,
  frameworks JSONB NOT NULL DEFAULT '[]'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, control_id)
);

CREATE INDEX idx_tenant_policies_tenant ON tenant_policies(tenant_id);
CREATE INDEX idx_tenant_policies_collector ON tenant_policies(collector_id);
