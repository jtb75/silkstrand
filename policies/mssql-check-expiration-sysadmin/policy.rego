package silkstrand.mssql.check_expiration_sysadmin

import rego.v1

default result := {
    "control_id": "mssql-check-expiration-sysadmin",
    "status": "fail",
    "severity": "high",
    "title": "Ensure CHECK_EXPIRATION is set to ON for sysadmin logins",
    "remediation": "ALTER LOGIN [<login_name>] WITH CHECK_EXPIRATION = ON;"
}

result := r if {
    count(input.facts.sysadmin_no_check_expiration) == 0
    r := {
        "control_id": "mssql-check-expiration-sysadmin",
        "status": "pass",
        "severity": "high",
        "title": "Ensure CHECK_EXPIRATION is set to ON for sysadmin logins",
        "evidence": {"sysadmin_no_check_expiration": input.facts.sysadmin_no_check_expiration}
    }
}
