#!/usr/bin/env python3
"""Control: mssql-public-role-permissions — Ensure only the default permissions are granted to the public server role"""

import json
import os
import sys

CONTROL_ID = "mssql-public-role-permissions"
TITLE = "Ensure only the default permissions are granted to the public server role"
SEVERITY = "medium"

QUERY = """SELECT * FROM master.sys.server_permissions WHERE (grantee_principal_id = SUSER_SID(N'public') AND state_desc LIKE 'GRANT%') AND NOT (state_desc = 'GRANT' AND [permission_name] = 'VIEW ANY DATABASE' AND class = 100) AND NOT (state_desc = 'GRANT' AND [permission_name] = 'CONNECT' AND class = 105 AND major_id = 2);"""


def _read_json(path):
    with open(path) as f:
        return json.load(f)


def main():
    config_path = os.environ.get("SILKSTRAND_TARGET_CONFIG")
    creds_path = os.environ.get("SILKSTRAND_CREDENTIALS")
    if not config_path:
        _emit("error", "SILKSTRAND_TARGET_CONFIG not set")
        return

    config = _read_json(config_path)
    creds = _read_json(creds_path) if creds_path else {}

    host = config.get("host", "localhost")
    port = int(config.get("port", 1433))
    database = config.get("database", "master")
    username = creds.get("username") or config.get("username", "sa")
    password = creds.get("password") or config.get("password", "")

    sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "bundles", "cis-mssql-2022", "content", "vendor"))
    import pytds  # noqa: E402

    try:
        conn = pytds.connect(
            server=host, port=port, database=database,
            user=username, password=password,
            autocommit=True, login_timeout=10, timeout=30,
        )
    except Exception as exc:
        _emit("error", f"connection failed: {exc}")
        return

    try:
        cursor = conn.cursor()
        cursor.execute(QUERY)
        cols = [d[0] for d in cursor.description] if cursor.description else []
        rows = [dict(zip(cols, r)) for r in cursor.fetchall()]
    except Exception as exc:
        _emit("error", f"query failed: {exc}")
        return
    finally:
        try:
            conn.close()
        except Exception:
            pass

    if rows:
        _emit("fail", f"{len(rows)} row(s) returned; expected none")
    else:
        _emit("pass", "no rows returned (as required)")


def _emit(status, detail):
    result = {
        "control_id": CONTROL_ID,
        "status": status,
        "severity": SEVERITY,
        "title": TITLE,
        "evidence": {"detail": detail},
        "remediation": "Refer to CIS Microsoft SQL Server 2022 Benchmark section for remediation steps.",
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
