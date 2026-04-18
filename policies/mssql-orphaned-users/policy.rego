package silkstrand.mssql.orphaned_users

import rego.v1

default result := {
    "control_id": "mssql-orphaned-users",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Orphaned Users are Dropped From SQL Server Databases",
    "remediation": "DROP USER [<orphaned_user>]; -- or remap with ALTER USER [<user>] WITH LOGIN = [<login>];"
}

result := r if {
    count(input.facts.orphaned_user_databases) == 0
    r := {
        "control_id": "mssql-orphaned-users",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Orphaned Users are Dropped From SQL Server Databases",
        "evidence": {"orphaned_user_databases": input.facts.orphaned_user_databases}
    }
}
