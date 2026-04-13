-- Allow deleting an agent even if targets still reference it.
-- Orphaned targets get agent_id=NULL and can be reassigned to a new agent.
ALTER TABLE targets DROP CONSTRAINT targets_agent_id_fkey;
ALTER TABLE targets
    ADD CONSTRAINT targets_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
