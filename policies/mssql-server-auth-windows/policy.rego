package silkstrand.mssql.server_auth_windows

import rego.v1

default result := {
    "control_id": "mssql-server-auth-windows",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Server Authentication is set to Windows Authentication Mode",
    "remediation": "Use SQL Server Management Studio > Server Properties > Security > Server Authentication > Windows Authentication Mode."
}

result := r if {
    input.facts.login_mode == 1
    r := {
        "control_id": "mssql-server-auth-windows",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Server Authentication is set to Windows Authentication Mode",
        "evidence": {"login_mode": input.facts.login_mode}
    }
}
