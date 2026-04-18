// MSSQL configuration collector for SilkStrand (ADR 011 PR 3).
//
// Reads credentials + target from stdin as JSON, connects to the MSSQL
// instance, runs all CIS-relevant queries, and prints a facts JSON
// document to stdout. Makes no pass/fail decisions — only reports facts.
//
// Exit 0 on success, non-zero on error.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

// Input is the JSON envelope read from stdin.
type Input struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// Output is the JSON envelope written to stdout.
type Output struct {
	CollectorID string         `json:"collector_id"`
	Facts       map[string]any `json:"facts"`
}

const (
	collectorID    = "mssql-config"
	connectTimeout = 15 * time.Second
	queryTimeout   = 30 * time.Second
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("[mssql-collector] ")

	var in Input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		log.Fatalf("reading stdin: %v", err)
	}
	if in.Host == "" {
		in.Host = "localhost"
	}
	if in.Port == 0 {
		in.Port = 1433
	}
	if in.Database == "" {
		in.Database = "master"
	}

	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&connection+timeout=%d",
		in.Username, in.Password, in.Host, in.Port, in.Database, int(connectTimeout.Seconds()))

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		log.Fatalf("opening connection: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("connecting to %s:%d: %v", in.Host, in.Port, err)
	}

	facts := make(map[string]any)

	collectSysConfigurations(db, facts)
	collectTrustworthyDatabases(db, facts)
	collectNonStandardPort(db, facts)
	collectHideInstance(db, facts)
	collectSAAccount(db, facts)
	collectAutoCloseContained(db, facts)
	collectIsClustered(db, facts)
	collectLoginMode(db, facts)
	collectGuestConnect(db, facts)
	collectOrphanedUsers(db, facts)
	collectSQLAuthContained(db, facts)
	collectPublicExtraPermissions(db, facts)
	collectBuiltinGroupLogins(db, facts)
	collectLocalWindowsGroupLogins(db, facts)
	collectMsdbPublicProxy(db, facts)
	collectMsdbAdminRoleMembers(db, facts)
	collectSysadminNoCheckExpiration(db, facts)
	collectLoginsNoCheckPolicy(db, facts)
	collectMaxErrorLogFiles(db, facts)
	collectDefaultTraceEnabled(db, facts)
	collectAuditLoginGaps(db, facts)
	collectWeakSymmetricKeys(db, facts)
	collectWeakAsymmetricKeys(db, facts)
	collectUnencryptedBackups(db, facts)
	collectNetworkEncryption(db, facts)
	collectUnencryptedUserDatabases(db, facts)

	out := Output{CollectorID: collectorID, Facts: facts}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		log.Fatalf("encoding output: %v", err)
	}
}

// ---------------------------------------------------------------------------
// sys.configurations helpers
// ---------------------------------------------------------------------------

// configOption names exactly as they appear in sys.configurations.
var configOptions = []string{
	"Ad Hoc Distributed Queries",
	"clr enabled",
	"clr strict security",
	"cross db ownership chaining",
	"Database Mail XPs",
	"Ole Automation Procedures",
	"remote access",
	"remote admin connections",
	"scan for startup procs",
	"default trace enabled",
}

// factKey maps a sys.configurations name to the fact key prefix.
var factKey = map[string]string{
	"Ad Hoc Distributed Queries": "ad_hoc_distributed_queries",
	"clr enabled":                "clr_enabled",
	"clr strict security":        "clr_strict_security",
	"cross db ownership chaining": "cross_db_ownership_chaining",
	"Database Mail XPs":           "database_mail_xps",
	"Ole Automation Procedures":   "ole_automation_procedures",
	"remote access":               "remote_access",
	"remote admin connections":     "remote_admin_connections",
	"scan for startup procs":       "scan_for_startup_procs",
	"default trace enabled":        "default_trace_enabled",
}

func collectSysConfigurations(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	for _, name := range configOptions {
		var configured, inUse int
		err := db.QueryRowContext(ctx,
			"SELECT CAST(value AS int), CAST(value_in_use AS int) FROM sys.configurations WHERE name = @p1",
			name,
		).Scan(&configured, &inUse)
		prefix := factKey[name]
		if err != nil {
			facts[prefix+"_configured"] = nil
			facts[prefix+"_in_use"] = nil
			continue
		}
		facts[prefix+"_configured"] = configured
		facts[prefix+"_in_use"] = inUse
	}
}

// ---------------------------------------------------------------------------
// Section 2 — Surface Area Reduction
// ---------------------------------------------------------------------------

func collectTrustworthyDatabases(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sys.databases WHERE is_trustworthy_on = 1 AND name != 'msdb'")
	facts["trustworthy_databases"] = collectStringColumn(rows, err)
}

func collectNonStandardPort(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var count int
	// This query may fail on non-Windows (Linux) SQL Server where
	// sys.dm_server_registry is not available. Treat errors as null.
	err := db.QueryRowContext(ctx, `
		IF (SELECT value_data FROM sys.dm_server_registry WHERE value_name = 'ListenOnAllIPs') = 1
			SELECT count(*) FROM sys.dm_server_registry
			WHERE registry_key LIKE '%IPAll%' AND value_name LIKE '%Tcp%' AND value_data = '1433'
		ELSE
			SELECT count(*) FROM sys.dm_server_registry
			WHERE value_name LIKE '%Tcp%' AND value_data = '1433'
	`).Scan(&count)
	if err != nil {
		facts["listen_on_default_port"] = nil
	} else {
		facts["listen_on_default_port"] = count
	}
}

func collectHideInstance(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var val int
	err := db.QueryRowContext(ctx, `
		DECLARE @getValue INT;
		EXEC master.sys.xp_instance_regread
			@rootkey    = N'HKEY_LOCAL_MACHINE',
			@key        = N'SOFTWARE\Microsoft\Microsoft SQL Server\MSSQLServer\SuperSocketNetLib',
			@value_name = N'HideInstance',
			@value      = @getValue OUTPUT;
		SELECT @getValue;
	`).Scan(&val)
	if err != nil {
		facts["hide_instance"] = nil
	} else {
		facts["hide_instance"] = val
	}
}

func collectSAAccount(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	// 2.13 — sa login disabled?
	var enabledCount int
	err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sys.server_principals WHERE sid = 0x01 AND is_disabled = 0",
	).Scan(&enabledCount)
	if err != nil {
		facts["sa_login_enabled"] = nil
	} else {
		facts["sa_login_enabled"] = enabledCount > 0
	}

	// 2.14 — sa login renamed?
	var saName string
	err = db.QueryRowContext(ctx,
		"SELECT name FROM sys.server_principals WHERE sid = 0x01",
	).Scan(&saName)
	if err != nil {
		facts["sa_login_name"] = nil
	} else {
		facts["sa_login_name"] = saName
	}

	// 2.16 — no login named 'sa'
	var saExists int
	err = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sys.server_principals WHERE name = 'sa'",
	).Scan(&saExists)
	if err != nil {
		facts["sa_login_exists"] = nil
	} else {
		facts["sa_login_exists"] = saExists > 0
	}
}

func collectAutoCloseContained(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sys.databases WHERE containment <> 0 AND is_auto_close_on = 1")
	facts["auto_close_contained_databases"] = collectStringColumn(rows, err)
}

func collectIsClustered(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var val int
	err := db.QueryRowContext(ctx,
		"SELECT CAST(SERVERPROPERTY('IsClustered') AS int)").Scan(&val)
	if err != nil {
		facts["is_clustered"] = nil
	} else {
		facts["is_clustered"] = val
	}
}

// ---------------------------------------------------------------------------
// Section 3 — Authentication and Authorization
// ---------------------------------------------------------------------------

func collectLoginMode(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var val int
	err := db.QueryRowContext(ctx,
		"SELECT CAST(SERVERPROPERTY('IsIntegratedSecurityOnly') AS int)").Scan(&val)
	if err != nil {
		facts["login_mode"] = nil
	} else {
		facts["login_mode"] = val
	}
}

func collectGuestConnect(db *sql.DB, facts map[string]any) {
	facts["guest_connect_databases"] = collectPerDatabase(db, `
		SELECT DB_NAME() AS db_name
		FROM sys.database_permissions
		WHERE grantee_principal_id = DATABASE_PRINCIPAL_ID('guest')
		AND state_desc LIKE 'GRANT%'
		AND permission_name = 'CONNECT'
	`)
}

func collectOrphanedUsers(db *sql.DB, facts map[string]any) {
	facts["orphaned_user_databases"] = collectPerDatabase(db, `
		SELECT DB_NAME() AS db_name
		FROM sys.database_principals AS dp
		LEFT JOIN sys.server_principals AS sp ON dp.sid = sp.sid
		WHERE sp.sid IS NULL
		AND dp.authentication_type_desc = 'INSTANCE'
	`)
}

func collectSQLAuthContained(db *sql.DB, facts map[string]any) {
	facts["sql_auth_contained_databases"] = collectPerDatabase(db, `
		SELECT DB_NAME() AS db_name
		FROM sys.database_principals
		WHERE name NOT IN ('dbo','Information_Schema','sys','guest')
		AND type IN ('U','S','G')
		AND authentication_type = 2
	`)
}

func collectPublicExtraPermissions(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT permission_name, state_desc, class_desc, major_id
		FROM master.sys.server_permissions
		WHERE (grantee_principal_id = SUSER_SID(N'public') AND state_desc LIKE 'GRANT%')
		AND NOT (state_desc = 'GRANT' AND permission_name = 'VIEW ANY DATABASE' AND class_desc = 'SERVER')
		AND NOT (state_desc = 'GRANT' AND permission_name = 'CONNECT' AND class_desc = 'ENDPOINT' AND major_id = 2)
		AND NOT (state_desc = 'GRANT' AND permission_name = 'CONNECT' AND class_desc = 'ENDPOINT' AND major_id = 3)
		AND NOT (state_desc = 'GRANT' AND permission_name = 'CONNECT' AND class_desc = 'ENDPOINT' AND major_id = 4)
		AND NOT (state_desc = 'GRANT' AND permission_name = 'CONNECT' AND class_desc = 'ENDPOINT' AND major_id = 5)
	`)
	facts["public_extra_permissions"] = collectRows(rows, err)
}

func collectBuiltinGroupLogins(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT pr.name, pe.permission_name, pe.state_desc
		FROM sys.server_principals pr
		JOIN sys.server_permissions pe ON pr.principal_id = pe.grantee_principal_id
		WHERE pr.name LIKE 'BUILTIN%'
	`)
	facts["builtin_group_logins"] = collectRows(rows, err)
}

func collectLocalWindowsGroupLogins(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT pr.name AS local_group_name, pe.permission_name, pe.state_desc
		FROM sys.server_principals pr
		JOIN sys.server_permissions pe ON pr.principal_id = pe.grantee_principal_id
		WHERE pr.type_desc = 'WINDOWS_GROUP'
		AND pr.name LIKE CAST(SERVERPROPERTY('MachineName') AS nvarchar) + '%'
	`)
	facts["local_windows_group_logins"] = collectRows(rows, err)
}

func collectMsdbPublicProxy(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT sp.name AS proxyname
		FROM msdb.dbo.sysproxylogin spl
		JOIN msdb.sys.database_principals dp ON dp.sid = spl.sid
		JOIN msdb.dbo.sysproxies sp ON sp.proxy_id = spl.proxy_id
		WHERE dp.principal_id = (SELECT principal_id FROM msdb.sys.database_principals WHERE name = 'public')
	`)
	facts["msdb_public_proxy_names"] = collectStringColumn(rows, err)
}

func collectMsdbAdminRoleMembers(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT r.name AS role_name, m.name AS member_name
		FROM msdb.sys.database_role_members AS drm
		INNER JOIN msdb.sys.database_principals AS r ON drm.role_principal_id = r.principal_id
		INNER JOIN msdb.sys.database_principals AS m ON drm.member_principal_id = m.principal_id
		WHERE r.name IN ('db_owner', 'db_securityadmin', 'db_ddladmin', 'db_datawriter')
		AND m.name <> 'dbo'
	`)
	facts["msdb_admin_role_members"] = collectRows(rows, err)
}

// ---------------------------------------------------------------------------
// Section 4 — Password Policies
// ---------------------------------------------------------------------------

func collectSysadminNoCheckExpiration(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT l.name, 'sysadmin membership' AS access_method
		FROM sys.sql_logins AS l
		WHERE IS_SRVROLEMEMBER('sysadmin', l.name) = 1
		AND l.is_expiration_checked <> 1
		UNION ALL
		SELECT l.name, 'CONTROL SERVER' AS access_method
		FROM sys.sql_logins AS l
		JOIN sys.server_permissions AS p ON l.principal_id = p.grantee_principal_id
		WHERE p.type = 'CL' AND p.state IN ('G', 'W')
		AND l.is_expiration_checked <> 1
	`)
	facts["sysadmin_no_check_expiration"] = collectRows(rows, err)
}

func collectLoginsNoCheckPolicy(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT name, is_disabled
		FROM sys.sql_logins
		WHERE is_policy_checked = 0
	`)
	facts["logins_no_check_policy"] = collectRows(rows, err)
}

// ---------------------------------------------------------------------------
// Section 5 — Auditing and Logging
// ---------------------------------------------------------------------------

func collectMaxErrorLogFiles(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var val int
	err := db.QueryRowContext(ctx, `
		DECLARE @NumErrorLogs int;
		EXEC master.sys.xp_instance_regread
			N'HKEY_LOCAL_MACHINE',
			N'Software\Microsoft\MSSQLServer\MSSQLServer',
			N'NumErrorLogs',
			@NumErrorLogs OUTPUT;
		SELECT ISNULL(@NumErrorLogs, -1);
	`).Scan(&val)
	if err != nil {
		facts["max_error_log_files"] = nil
	} else {
		facts["max_error_log_files"] = val
	}
}

func collectDefaultTraceEnabled(db *sql.DB, facts map[string]any) {
	// Already collected via collectSysConfigurations ("default trace enabled")
	// but we keep the name consistent — the sys.configurations collector
	// writes default_trace_enabled_configured and default_trace_enabled_in_use.
}

func collectAuditLoginGaps(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		WITH required_groups AS (
			SELECT 'FAILED_LOGIN_GROUP' AS action_name
			UNION ALL SELECT 'SUCCESSFUL_LOGIN_GROUP'
		),
		matching AS (
			SELECT
				SAD.audit_action_name,
				S.is_state_enabled AS audit_enabled,
				SA.is_state_enabled AS spec_enabled,
				SAD.audited_result
			FROM sys.server_audit_specification_details AS SAD
			JOIN sys.server_audit_specifications AS SA
				ON SAD.server_specification_id = SA.server_specification_id
			JOIN sys.server_audits AS S
				ON SA.audit_guid = S.audit_guid
		)
		SELECT rg.action_name AS missing_or_noncompliant
		FROM required_groups rg
		WHERE NOT EXISTS (
			SELECT 1 FROM matching m
			WHERE m.audit_action_name = rg.action_name
				AND m.audit_enabled = 1
				AND m.spec_enabled = 1
				AND m.audited_result LIKE '%SUCCESS%'
				AND m.audited_result LIKE '%FAILURE%'
		)
	`)
	facts["audit_login_gaps"] = collectStringColumn(rows, err)
}

// ---------------------------------------------------------------------------
// Section 7 — Encryption
// ---------------------------------------------------------------------------

func collectWeakSymmetricKeys(db *sql.DB, facts map[string]any) {
	facts["weak_symmetric_key_databases"] = collectPerDatabase(db, `
		SELECT DB_NAME() AS db_name
		FROM sys.symmetric_keys
		WHERE algorithm_desc NOT IN ('AES_128','AES_192','AES_256')
		AND DB_ID() > 4
	`)
}

func collectWeakAsymmetricKeys(db *sql.DB, facts map[string]any) {
	facts["weak_asymmetric_key_databases"] = collectPerDatabase(db, `
		SELECT DB_NAME() AS db_name
		FROM sys.asymmetric_keys
		WHERE key_length < 2048
		AND DB_ID() > 4
	`)
}

func collectUnencryptedBackups(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM msdb.dbo.backupset b
		INNER JOIN sys.databases d ON b.database_name = d.name
		WHERE b.key_algorithm IS NULL
			AND b.encryptor_type IS NULL
			AND d.is_encrypted = 0
	`).Scan(&count)
	if err != nil {
		facts["unencrypted_backups_exist"] = nil
	} else {
		facts["unencrypted_backups_exist"] = count > 0
	}
}

func collectNetworkEncryption(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT CAST(encrypt_option AS varchar(10)) AS encrypt_option
		FROM sys.dm_exec_connections c
		WHERE net_transport <> 'Shared memory'
			AND c.endpoint_id NOT IN (
				SELECT endpoint_id FROM sys.database_mirroring_endpoints
				WHERE encryption_algorithm IS NOT NULL
			)
	`)
	facts["network_encryption_options"] = collectStringColumn(rows, err)
}

func collectUnencryptedUserDatabases(db *sql.DB, facts map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sys.databases WHERE database_id > 4 AND is_encrypted != 1")
	facts["unencrypted_user_databases"] = collectStringColumn(rows, err)
}

// ---------------------------------------------------------------------------
// Per-database iteration helpers
// ---------------------------------------------------------------------------

// userDatabases returns names of online user databases (database_id > 4).
func userDatabases(db *sql.DB) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT name FROM sys.databases
		WHERE database_id > 4 AND state = 0
		AND name NOT IN ('master','tempdb','model','msdb','rdsadmin')
	`)
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scanning database name: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// collectPerDatabase runs query in every user database via USE [db] and
// returns the list of database names where the query returned at least
// one row. The query must select a column named db_name.
func collectPerDatabase(db *sql.DB, query string) []string {
	dbs, err := userDatabases(db)
	if err != nil {
		return nil
	}

	var offenders []string
	for _, dbName := range dbs {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		// USE [dbName]; must be a separate statement executed first.
		_, err := db.ExecContext(ctx, fmt.Sprintf("USE [%s]", dbName))
		if err != nil {
			cancel()
			continue
		}
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			cancel()
			continue
		}
		if rows.Next() {
			offenders = append(offenders, dbName)
		}
		rows.Close()
		cancel()
	}
	// Reset to master
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	_, _ = db.ExecContext(ctx, "USE [master]")

	if offenders == nil {
		return []string{}
	}
	return offenders
}

// ---------------------------------------------------------------------------
// Row collection helpers
// ---------------------------------------------------------------------------

// collectStringColumn reads all rows from a query result, scanning the
// first column as a string. Returns an empty slice on error or no rows.
func collectStringColumn(rows *sql.Rows, err error) []string {
	if err != nil {
		return []string{}
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		result = append(result, s)
	}
	if result == nil {
		return []string{}
	}
	return result
}

// collectRows reads all rows from a query result and returns them as a
// slice of maps (column name -> value). Returns an empty slice on error.
func collectRows(rows *sql.Rows, err error) []map[string]any {
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return []map[string]any{}
	}

	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = v
			}
		}
		result = append(result, row)
	}
	if result == nil {
		return []map[string]any{}
	}
	return result
}
