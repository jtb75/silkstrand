package silkstrand.mssql.sa_disabled

import rego.v1

default result := {
    "control_id": "mssql-sa-disabled",
    "status": "fail",
    "severity": "high",
    "title": "Ensure the sa Login Account is set to Disabled",
    "remediation": "ALTER LOGIN sa DISABLE;"
}

result := r if {
    not input.facts.sa_login_enabled
    r := {
        "control_id": "mssql-sa-disabled",
        "status": "pass",
        "severity": "high",
        "title": "Ensure the sa Login Account is set to Disabled",
        "evidence": {"sa_login_enabled": input.facts.sa_login_enabled}
    }
}
