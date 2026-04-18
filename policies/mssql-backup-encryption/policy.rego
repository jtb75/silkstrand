package silkstrand.mssql.backup_encryption

import rego.v1

default result := {
    "control_id": "mssql-backup-encryption",
    "status": "fail",
    "severity": "high",
    "title": "Ensure Database Backups are Encrypted",
    "remediation": "Use BACKUP DATABASE ... WITH ENCRYPTION (ALGORITHM = AES_256, SERVER CERTIFICATE = ...);"
}

result := r if {
    not input.facts.unencrypted_backups_exist
    r := {
        "control_id": "mssql-backup-encryption",
        "status": "pass",
        "severity": "high",
        "title": "Ensure Database Backups are Encrypted",
        "evidence": {"unencrypted_backups_exist": input.facts.unencrypted_backups_exist}
    }
}
