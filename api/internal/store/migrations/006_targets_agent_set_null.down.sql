ALTER TABLE targets DROP CONSTRAINT targets_agent_id_fkey;
ALTER TABLE targets
    ADD CONSTRAINT targets_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id);
