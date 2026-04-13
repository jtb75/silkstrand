-- Revert engine-specific values back to the legacy generic "database".
-- All engine-specific types collapse into "database" on rollback;
-- the precise original engine is not recoverable, but it is always
-- inferrable from target.config at runtime.
UPDATE targets
   SET type = 'database'
 WHERE type IN ('postgresql', 'aurora_postgresql', 'mssql', 'mongodb',
                'mysql', 'aurora_mysql');
