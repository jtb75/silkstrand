-- One credential per target for the MVP credential UI. Existing rows (if any)
-- collapse to the latest created.
DELETE FROM credentials c
 WHERE c.id NOT IN (
   SELECT DISTINCT ON (target_id) id
     FROM credentials
    ORDER BY target_id, created_at DESC
 );

CREATE UNIQUE INDEX credentials_target_unique ON credentials(target_id);
