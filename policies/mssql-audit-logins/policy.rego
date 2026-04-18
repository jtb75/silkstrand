package silkstrand.mssql.audit_logins

import rego.v1

default result := {
    "control_id": "mssql-audit-logins",
    "status": "fail",
    "severity": "high",
    "title": "Ensure SQL Server Audit captures both failed and successful logins",
    "remediation": "Create or update a server audit specification to include FAILED_LOGIN_GROUP and SUCCESSFUL_LOGIN_GROUP."
}

result := r if {
    count(input.facts.audit_login_gaps) == 0
    r := {
        "control_id": "mssql-audit-logins",
        "status": "pass",
        "severity": "high",
        "title": "Ensure SQL Server Audit captures both failed and successful logins",
        "evidence": {"audit_login_gaps": input.facts.audit_login_gaps}
    }
}
