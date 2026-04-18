package silkstrand.mssql.sql_auth_contained_dbs

import rego.v1

default result := {
    "control_id": "mssql-sql-auth-contained-dbs",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure SQL Authentication is not used in contained databases",
    "remediation": "Migrate contained database users to Windows Authentication."
}

result := r if {
    count(input.facts.sql_auth_contained_databases) == 0
    r := {
        "control_id": "mssql-sql-auth-contained-dbs",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure SQL Authentication is not used in contained databases",
        "evidence": {"sql_auth_contained_databases": input.facts.sql_auth_contained_databases}
    }
}
