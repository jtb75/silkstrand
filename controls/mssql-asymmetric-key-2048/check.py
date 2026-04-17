#!/usr/bin/env python3
"""Control: mssql-asymmetric-key-2048 — Ensure Asymmetric Key Size is 2048 or greater"""

import json
import os
import sys

CONTROL_ID = "mssql-asymmetric-key-2048"
TITLE = "Ensure Asymmetric Key Size is 2048 or greater"
SEVERITY = "high"

QUERY = """SELECT db_id() AS db_id, name AS key_name, key_length FROM sys.asymmetric_keys WHERE key_length < 2048 AND db_id() > 4;"""


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
        # Get user databases
        cursor.execute(
            "SELECT name FROM sys.databases "
            "WHERE database_id > 4 AND state = 0 "
            "AND name NOT IN ('master','tempdb','model','msdb','rdsadmin');"
        )
        dbs = [r[0] for r in cursor.fetchall()]

        if not dbs:
            _emit("pass", "no user databases present")
            return

        offenders = []
        for db in dbs:
            try:
                cursor.execute(f"USE [{db}];")
                cursor.execute(QUERY)
                cols = [d[0] for d in cursor.description] if cursor.description else []
                rows = [dict(zip(cols, r)) for r in cursor.fetchall()]
                if rows:
                    offenders.append(f"{db} ({len(rows)})")
            except Exception as exc:
                _emit("error", f"query failed in database {db!r}: {exc}")
                return

        if offenders:
            _emit("fail", "row(s) returned in: " + ", ".join(offenders))
        else:
            _emit("pass", f"clean across {len(dbs)} user database(s)")
    except Exception as exc:
        _emit("error", f"query failed: {exc}")
    finally:
        try:
            conn.close()
        except Exception:
            pass


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
