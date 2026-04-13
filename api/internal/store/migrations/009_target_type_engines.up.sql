-- Target types become engine-specific. The legacy value "database" was
-- only ever Postgres (the only DB we supported). Migrate it forward so
-- the per-type prober / bundle matching / UI form can dispatch cleanly.
UPDATE targets
   SET type = 'postgresql'
 WHERE type = 'database';
