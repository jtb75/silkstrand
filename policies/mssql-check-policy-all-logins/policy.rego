package silkstrand.mssql.check_policy_all_logins

import rego.v1

default result := {
    "control_id": "mssql-check-policy-all-logins",
    "status": "fail",
    "severity": "high",
    "title": "Ensure CHECK_POLICY is set to ON for All SQL Authenticated Logins",
    "remediation": "ALTER LOGIN [<login_name>] WITH CHECK_POLICY = ON;"
}

result := r if {
    count(input.facts.logins_no_check_policy) == 0
    r := {
        "control_id": "mssql-check-policy-all-logins",
        "status": "pass",
        "severity": "high",
        "title": "Ensure CHECK_POLICY is set to ON for All SQL Authenticated Logins",
        "evidence": {"logins_no_check_policy": input.facts.logins_no_check_policy}
    }
}
