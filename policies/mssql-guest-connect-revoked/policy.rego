package silkstrand.mssql.guest_connect_revoked

import rego.v1

default result := {
    "control_id": "mssql-guest-connect-revoked",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure CONNECT permissions on guest user is Revoked",
    "remediation": "USE [<db_name>]; REVOKE CONNECT FROM guest;"
}

result := r if {
    count(input.facts.guest_connect_databases) == 0
    r := {
        "control_id": "mssql-guest-connect-revoked",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure CONNECT permissions on guest user is Revoked",
        "evidence": {"guest_connect_databases": input.facts.guest_connect_databases}
    }
}
