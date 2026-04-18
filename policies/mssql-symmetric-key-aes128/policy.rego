package silkstrand.mssql.symmetric_key_aes128

import rego.v1

default result := {
    "control_id": "mssql-symmetric-key-aes128",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Symmetric Key encryption uses AES_128 or higher",
    "remediation": "Recreate symmetric keys using AES_128, AES_192, or AES_256 algorithm."
}

result := r if {
    count(input.facts.weak_symmetric_key_databases) == 0
    r := {
        "control_id": "mssql-symmetric-key-aes128",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Symmetric Key encryption uses AES_128 or higher",
        "evidence": {"weak_symmetric_key_databases": input.facts.weak_symmetric_key_databases}
    }
}
