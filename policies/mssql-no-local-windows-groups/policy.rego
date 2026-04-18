package silkstrand.mssql.no_local_windows_groups

import rego.v1

default result := {
    "control_id": "mssql-no-local-windows-groups",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Windows local groups are not SQL Logins",
    "remediation": "DROP LOGIN [<MACHINE>\\<group>]; -- for each local Windows group login"
}

result := r if {
    count(input.facts.local_windows_group_logins) == 0
    r := {
        "control_id": "mssql-no-local-windows-groups",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Windows local groups are not SQL Logins",
        "evidence": {"local_windows_group_logins": input.facts.local_windows_group_logins}
    }
}
