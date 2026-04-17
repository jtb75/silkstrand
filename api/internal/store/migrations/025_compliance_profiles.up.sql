CREATE TABLE compliance_profiles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT,
  base_framework TEXT,
  status TEXT NOT NULL DEFAULT 'draft',
  version INT NOT NULL DEFAULT 1,
  bundle_id UUID REFERENCES bundles(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by UUID,
  UNIQUE (tenant_id, name)
);

CREATE TABLE profile_controls (
  profile_id UUID NOT NULL REFERENCES compliance_profiles(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,
  PRIMARY KEY (profile_id, control_id)
);

CREATE INDEX idx_profiles_tenant ON compliance_profiles(tenant_id);
CREATE INDEX idx_profile_controls_control ON profile_controls(control_id);
