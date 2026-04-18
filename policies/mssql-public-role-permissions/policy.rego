package silkstrand.mssql.public_role_permissions

import rego.v1

default result := {
    "control_id": "mssql-public-role-permissions",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure only the default permissions are granted to the public server role",
    "remediation": "REVOKE <permission> FROM public; -- for each non-default permission"
}

result := r if {
    count(input.facts.public_extra_permissions) == 0
    r := {
        "control_id": "mssql-public-role-permissions",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure only the default permissions are granted to the public server role",
        "evidence": {"public_extra_permissions": input.facts.public_extra_permissions}
    }
}
