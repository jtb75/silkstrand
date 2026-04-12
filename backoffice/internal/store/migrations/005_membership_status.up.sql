ALTER TABLE memberships
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'suspended'));

CREATE INDEX idx_memberships_user_active ON memberships(user_id) WHERE status = 'active';
