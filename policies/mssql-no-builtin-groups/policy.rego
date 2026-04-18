package silkstrand.mssql.no_builtin_groups

import rego.v1

default result := {
    "control_id": "mssql-no-builtin-groups",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Windows BUILTIN groups are not SQL Logins",
    "remediation": "DROP LOGIN [BUILTIN\\<group>]; -- for each BUILTIN group login"
}

result := r if {
    count(input.facts.builtin_group_logins) == 0
    r := {
        "control_id": "mssql-no-builtin-groups",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Windows BUILTIN groups are not SQL Logins",
        "evidence": {"builtin_group_logins": input.facts.builtin_group_logins}
    }
}
