-- Allow deleting a target by cascading to its credentials and scans.
-- Matches the pattern already applied for agents (migrations 006/007).
-- Credentials: a target's credential is scoped to the target; deleting
-- the target should remove the credential.
ALTER TABLE credentials DROP CONSTRAINT credentials_target_id_fkey;
ALTER TABLE credentials
    ADD CONSTRAINT credentials_target_id_fkey
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE;

-- Scans: scan history tied to a deleted target is also removed.
-- scan_results already cascades off scans, so that chain completes.
ALTER TABLE scans DROP CONSTRAINT scans_target_id_fkey;
ALTER TABLE scans
    ADD CONSTRAINT scans_target_id_fkey
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE;
