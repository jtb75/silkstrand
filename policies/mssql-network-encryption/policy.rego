package silkstrand.mssql.network_encryption

import rego.v1

default result := {
    "control_id": "mssql-network-encryption",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Network Encryption is Configured and Enabled",
    "remediation": "Enable Force Encryption in SQL Server Configuration Manager and configure a TLS certificate."
}

# Pass if all non-shared-memory connections use encryption (every option is TRUE)
result := r if {
    count(input.facts.network_encryption_options) > 0
    every opt in input.facts.network_encryption_options {
        opt == "TRUE"
    }
    r := {
        "control_id": "mssql-network-encryption",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Network Encryption is Configured and Enabled",
        "evidence": {"network_encryption_options": input.facts.network_encryption_options}
    }
}
