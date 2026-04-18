package silkstrand.mssql.tde

import rego.v1

default result := {
    "control_id": "mssql-tde",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Databases are Encrypted with TDE",
    "remediation": "CREATE DATABASE ENCRYPTION KEY WITH ALGORITHM = AES_256 ENCRYPTION BY SERVER CERTIFICATE <cert>; ALTER DATABASE [<db>] SET ENCRYPTION ON;"
}

result := r if {
    count(input.facts.unencrypted_user_databases) == 0
    r := {
        "control_id": "mssql-tde",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Databases are Encrypted with TDE",
        "evidence": {"unencrypted_user_databases": input.facts.unencrypted_user_databases}
    }
}
