DROP INDEX IF EXISTS idx_memberships_user_active;
ALTER TABLE memberships DROP COLUMN IF EXISTS status;
