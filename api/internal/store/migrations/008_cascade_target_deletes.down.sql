ALTER TABLE credentials DROP CONSTRAINT credentials_target_id_fkey;
ALTER TABLE credentials
    ADD CONSTRAINT credentials_target_id_fkey
    FOREIGN KEY (target_id) REFERENCES targets(id);

ALTER TABLE scans DROP CONSTRAINT scans_target_id_fkey;
ALTER TABLE scans
    ADD CONSTRAINT scans_target_id_fkey
    FOREIGN KEY (target_id) REFERENCES targets(id);
