package silkstrand.mssql.clr_enabled

import rego.v1

default result := {
    "control_id": "mssql-clr-enabled",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure CLR Enabled is set to 0",
    "remediation": "EXEC sp_configure 'clr enabled', 0; RECONFIGURE;"
}

result := r if {
    input.facts.clr_enabled_in_use == 0
    r := {
        "control_id": "mssql-clr-enabled",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure CLR Enabled is set to 0",
        "evidence": {"clr_enabled_in_use": input.facts.clr_enabled_in_use}
    }
}
