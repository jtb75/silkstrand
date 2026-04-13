ALTER TABLE scans DROP CONSTRAINT scans_agent_id_fkey;
ALTER TABLE scans
    ADD CONSTRAINT scans_agent_id_fkey
    FOREIGN KEY (agent_id) REFERENCES agents(id);
