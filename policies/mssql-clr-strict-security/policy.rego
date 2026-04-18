package silkstrand.mssql.clr_strict_security

import rego.v1

default result := {
    "control_id": "mssql-clr-strict-security",
    "status": "fail",
    "severity": "high",
    "title": "Ensure clr strict security is set to 1",
    "remediation": "EXEC sp_configure 'clr strict security', 1; RECONFIGURE;"
}

result := r if {
    input.facts.clr_strict_security_in_use == 1
    r := {
        "control_id": "mssql-clr-strict-security",
        "status": "pass",
        "severity": "high",
        "title": "Ensure clr strict security is set to 1",
        "evidence": {"clr_strict_security_in_use": input.facts.clr_strict_security_in_use}
    }
}
