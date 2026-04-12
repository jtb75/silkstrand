CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_type TEXT NOT NULL,       -- 'admin' | 'tenant_user' | 'system'
    actor_id TEXT,                  -- UUID of admin_users or users row, or null for system
    actor_email TEXT,               -- denormalized for display after user deletion
    action TEXT NOT NULL,           -- e.g. 'tenant.create', 'member.invite'
    target_type TEXT,               -- 'tenant' | 'user' | 'membership' | 'invitation' | 'data_center'
    target_id TEXT,
    tenant_id UUID,                 -- context if the action is scoped to a tenant
    ip TEXT,                        -- client IP at time of action (if available)
    metadata JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_audit_log_occurred ON audit_log(occurred_at DESC);
CREATE INDEX idx_audit_log_tenant ON audit_log(tenant_id, occurred_at DESC);
CREATE INDEX idx_audit_log_actor ON audit_log(actor_id);
CREATE INDEX idx_audit_log_action ON audit_log(action);
