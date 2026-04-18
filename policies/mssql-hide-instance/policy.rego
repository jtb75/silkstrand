package silkstrand.mssql.hide_instance

import rego.v1

default result := {
    "control_id": "mssql-hide-instance",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Hide Instance option is set to Yes",
    "remediation": "Enable Hide Instance in SQL Server Configuration Manager > SQL Server Network Configuration > Protocols > Properties > Hide Instance = Yes."
}

result := r if {
    input.facts.hide_instance == 1
    r := {
        "control_id": "mssql-hide-instance",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Hide Instance option is set to Yes",
        "evidence": {"hide_instance": input.facts.hide_instance}
    }
}
