package silkstrand.mssql.asymmetric_key_2048

import rego.v1

default result := {
    "control_id": "mssql-asymmetric-key-2048",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Asymmetric Key Size is 2048 or greater",
    "remediation": "Recreate asymmetric keys with a minimum key length of 2048 bits."
}

result := r if {
    count(input.facts.weak_asymmetric_key_databases) == 0
    r := {
        "control_id": "mssql-asymmetric-key-2048",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Asymmetric Key Size is 2048 or greater",
        "evidence": {"weak_asymmetric_key_databases": input.facts.weak_asymmetric_key_databases}
    }
}
