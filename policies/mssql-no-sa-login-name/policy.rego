package silkstrand.mssql.no_sa_login_name

import rego.v1

default result := {
    "control_id": "mssql-no-sa-login-name",
    "status": "fail",
    "severity": "high",
    "title": "Ensure no login exists with the name sa",
    "remediation": "ALTER LOGIN sa WITH NAME = [<new_name>];"
}

result := r if {
    not input.facts.sa_login_exists
    r := {
        "control_id": "mssql-no-sa-login-name",
        "status": "pass",
        "severity": "high",
        "title": "Ensure no login exists with the name sa",
        "evidence": {"sa_login_exists": input.facts.sa_login_exists}
    }
}
