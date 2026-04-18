package silkstrand.mssql.default_trace_enabled

import rego.v1

default result := {
    "control_id": "mssql-default-trace-enabled",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Default Trace Enabled is set to 1",
    "remediation": "EXEC sp_configure 'default trace enabled', 1; RECONFIGURE;"
}

result := r if {
    input.facts.default_trace_enabled_in_use == 1
    r := {
        "control_id": "mssql-default-trace-enabled",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Default Trace Enabled is set to 1",
        "evidence": {"default_trace_enabled_in_use": input.facts.default_trace_enabled_in_use}
    }
}
