package silkstrand.mssql.non_standard_port

import rego.v1

default result := {
    "control_id": "mssql-non-standard-port",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure SQL Server is configured to use non-standard ports",
    "remediation": "Change the default port from 1433 in SQL Server Configuration Manager."
}

result := r if {
    input.facts.listen_on_default_port == 0
    r := {
        "control_id": "mssql-non-standard-port",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure SQL Server is configured to use non-standard ports",
        "evidence": {"listen_on_default_port": input.facts.listen_on_default_port}
    }
}
