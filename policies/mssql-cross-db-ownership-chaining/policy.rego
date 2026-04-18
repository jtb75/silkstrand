package silkstrand.mssql.cross_db_ownership_chaining

import rego.v1

default result := {
    "control_id": "mssql-cross-db-ownership-chaining",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Cross DB Ownership Chaining is set to 0",
    "remediation": "EXEC sp_configure 'cross db ownership chaining', 0; RECONFIGURE;"
}

result := r if {
    input.facts.cross_db_ownership_chaining_in_use == 0
    r := {
        "control_id": "mssql-cross-db-ownership-chaining",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Cross DB Ownership Chaining is set to 0",
        "evidence": {"cross_db_ownership_chaining_in_use": input.facts.cross_db_ownership_chaining_in_use}
    }
}
