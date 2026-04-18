package silkstrand.mssql.ole_automation_procedures

import rego.v1

default result := {
    "control_id": "mssql-ole-automation-procedures",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Ole Automation Procedures is set to 0",
    "remediation": "EXEC sp_configure 'Ole Automation Procedures', 0; RECONFIGURE;"
}

result := r if {
    input.facts.ole_automation_procedures_in_use == 0
    r := {
        "control_id": "mssql-ole-automation-procedures",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Ole Automation Procedures is set to 0",
        "evidence": {"ole_automation_procedures_in_use": input.facts.ole_automation_procedures_in_use}
    }
}
