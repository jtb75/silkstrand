package silkstrand.mssql.remote_access

import rego.v1

default result := {
    "control_id": "mssql-remote-access",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Remote Access is set to 0",
    "remediation": "EXEC sp_configure 'remote access', 0; RECONFIGURE;"
}

result := r if {
    input.facts.remote_access_in_use == 0
    r := {
        "control_id": "mssql-remote-access",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Remote Access is set to 0",
        "evidence": {"remote_access_in_use": input.facts.remote_access_in_use}
    }
}
