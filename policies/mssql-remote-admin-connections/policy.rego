package silkstrand.mssql.remote_admin_connections

import rego.v1

default result := {
    "control_id": "mssql-remote-admin-connections",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Remote Admin Connections is set to 0",
    "remediation": "EXEC sp_configure 'remote admin connections', 0; RECONFIGURE;"
}

# Pass if remote admin connections is disabled (non-clustered)
result := r if {
    input.facts.remote_admin_connections_in_use == 0
    r := {
        "control_id": "mssql-remote-admin-connections",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Remote Admin Connections is set to 0",
        "evidence": {
            "remote_admin_connections_in_use": input.facts.remote_admin_connections_in_use,
            "is_clustered": input.facts.is_clustered
        }
    }
}

# Pass if clustered — CIS allows remote admin on clustered instances
result := r if {
    input.facts.is_clustered == 1
    r := {
        "control_id": "mssql-remote-admin-connections",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Remote Admin Connections is set to 0",
        "evidence": {
            "remote_admin_connections_in_use": input.facts.remote_admin_connections_in_use,
            "is_clustered": input.facts.is_clustered
        }
    }
}
