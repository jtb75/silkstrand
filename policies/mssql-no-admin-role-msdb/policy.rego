package silkstrand.mssql.no_admin_role_msdb

import rego.v1

default result := {
    "control_id": "mssql-no-admin-role-msdb",
    "status": "fail",
    "severity": "high",
    "title": "Ensure no admin role membership in MSDB database",
    "remediation": "USE msdb; ALTER ROLE [<role_name>] DROP MEMBER [<member_name>];"
}

result := r if {
    count(input.facts.msdb_admin_role_members) == 0
    r := {
        "control_id": "mssql-no-admin-role-msdb",
        "status": "pass",
        "severity": "high",
        "title": "Ensure no admin role membership in MSDB database",
        "evidence": {"msdb_admin_role_members": input.facts.msdb_admin_role_members}
    }
}
