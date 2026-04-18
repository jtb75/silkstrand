package silkstrand.mssql.scan_for_startup_procs

import rego.v1

default result := {
    "control_id": "mssql-scan-for-startup-procs",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Scan For Startup Procs is set to 0",
    "remediation": "EXEC sp_configure 'scan for startup procs', 0; RECONFIGURE;"
}

result := r if {
    input.facts.scan_for_startup_procs_in_use == 0
    r := {
        "control_id": "mssql-scan-for-startup-procs",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Scan For Startup Procs is set to 0",
        "evidence": {"scan_for_startup_procs_in_use": input.facts.scan_for_startup_procs_in_use}
    }
}
