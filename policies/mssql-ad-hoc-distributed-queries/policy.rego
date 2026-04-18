package silkstrand.mssql.ad_hoc_distributed_queries

import rego.v1

default result := {
    "control_id": "mssql-ad-hoc-distributed-queries",
    "status": "fail",
    "severity": "medium",
    "title": "Ensure Ad Hoc Distributed Queries is set to 0",
    "remediation": "EXEC sp_configure 'Ad Hoc Distributed Queries', 0; RECONFIGURE;"
}

result := r if {
    input.facts.ad_hoc_distributed_queries_in_use == 0
    r := {
        "control_id": "mssql-ad-hoc-distributed-queries",
        "status": "pass",
        "severity": "medium",
        "title": "Ensure Ad Hoc Distributed Queries is set to 0",
        "evidence": {"ad_hoc_distributed_queries_in_use": input.facts.ad_hoc_distributed_queries_in_use}
    }
}
