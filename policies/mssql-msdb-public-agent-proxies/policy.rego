package silkstrand.mssql.msdb_public_agent_proxies

import rego.v1

default result := {
    "control_id": "mssql-msdb-public-agent-proxies",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure the public role in msdb is not granted access to SQL Agent proxies",
    "remediation": "EXEC msdb.dbo.sp_revoke_login_from_proxy @name = N'public', @proxy_name = N'<proxy_name>';"
}

result := r if {
    count(input.facts.msdb_public_proxy_names) == 0
    r := {
        "control_id": "mssql-msdb-public-agent-proxies",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure the public role in msdb is not granted access to SQL Agent proxies",
        "evidence": {"msdb_public_proxy_names": input.facts.msdb_public_proxy_names}
    }
}
