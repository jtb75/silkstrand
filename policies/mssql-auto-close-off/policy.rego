package silkstrand.mssql.auto_close_off

import rego.v1

default result := {
    "control_id": "mssql-auto-close-off",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure AUTO_CLOSE is set to OFF on contained databases",
    "remediation": "ALTER DATABASE [<db_name>] SET AUTO_CLOSE OFF;"
}

result := r if {
    count(input.facts.auto_close_contained_databases) == 0
    r := {
        "control_id": "mssql-auto-close-off",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure AUTO_CLOSE is set to OFF on contained databases",
        "evidence": {"auto_close_contained_databases": input.facts.auto_close_contained_databases}
    }
}
