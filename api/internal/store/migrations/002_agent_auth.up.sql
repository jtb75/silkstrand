ALTER TABLE agents ADD COLUMN key_hash TEXT;
ALTER TABLE agents ADD COLUMN next_key_hash TEXT;
ALTER TABLE agents ADD COLUMN key_rotated_at TIMESTAMPTZ;
