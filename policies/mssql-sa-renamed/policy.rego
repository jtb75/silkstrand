package silkstrand.mssql.sa_renamed

import rego.v1

default result := {
    "control_id": "mssql-sa-renamed",
    "status": "fail",
    "severity": "high",
    "title": "Ensure the sa Login Account has been renamed",
    "remediation": "ALTER LOGIN sa WITH NAME = [<new_name>];"
}

result := r if {
    input.facts.sa_login_name != "sa"
    r := {
        "control_id": "mssql-sa-renamed",
        "status": "pass",
        "severity": "high",
        "title": "Ensure the sa Login Account has been renamed",
        "evidence": {"sa_login_name": input.facts.sa_login_name}
    }
}
