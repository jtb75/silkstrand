package silkstrand.mssql.trustworthy_off

import rego.v1

default result := {
    "control_id": "mssql-trustworthy-off",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Trustworthy Database Property is set to Off",
    "remediation": "ALTER DATABASE [<db_name>] SET TRUSTWORTHY OFF;"
}

result := r if {
    count(input.facts.trustworthy_databases) == 0
    r := {
        "control_id": "mssql-trustworthy-off",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Trustworthy Database Property is set to Off",
        "evidence": {"trustworthy_databases": input.facts.trustworthy_databases}
    }
}
