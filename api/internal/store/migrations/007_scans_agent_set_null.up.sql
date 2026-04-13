-- Allow deleting an agent even if historical scans still reference it.
-- Orphaned scans get agent_id=NULL but the scan history is preserved.
ALTER TABLE scans DROP CONSTRAINT scans_agent_id_fkey;
ALTER TABLE scans
    ADD CONSTRAINT scans_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
