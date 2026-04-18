package silkstrand.mssql.database_mail_xps

import rego.v1

default result := {
    "control_id": "mssql-database-mail-xps",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Database Mail XPs is set to 0",
    "remediation": "EXEC sp_configure 'Database Mail XPs', 0; RECONFIGURE;"
}

result := r if {
    input.facts.database_mail_xps_in_use == 0
    r := {
        "control_id": "mssql-database-mail-xps",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Database Mail XPs is set to 0",
        "evidence": {"database_mail_xps_in_use": input.facts.database_mail_xps_in_use}
    }
}
