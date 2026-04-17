#!/usr/bin/env python3
"""Control: mssql-clr-enabled — Ensure CLR Enabled is set to 0"""

import json
import os
import re
import sys

CONTROL_ID = "mssql-clr-enabled"
TITLE = "Ensure CLR Enabled is set to 0"
SEVERITY = "medium"

QUERY = """SELECT name, CAST(value AS int) AS value_configured, CAST(value_in_use AS int) AS value_in_use FROM sys.configurations WHERE name = 'clr enabled';"""

ASSERTIONS = [
    ("value_in_use", "equals", "0"),
]


def _coerce(x):
    if isinstance(x, bool):
        return x
    if isinstance(x, (int, float)):
        return x
    if isinstance(x, str):
        s = x.strip()
        try:
            return int(s)
        except ValueError:
            try:
                return float(s)
            except ValueError:
                return s
    return x


OPS = {
    "equals": lambda a, b: _coerce(a) == _coerce(b),
    "not_equals": lambda a, b: _coerce(a) != _coerce(b),
    "greater_than_or_equal": lambda a, b: _coerce(a) >= _coerce(b),
    "pattern_match": lambda a, b: re.search(b, str(a)) is not None,
}


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

    if not rows:
        _emit("fail", "query returned no rows")
        return

    for row in rows:
        for field, op, expected in ASSERTIONS:
            if field not in row:
                _emit("fail", f"field {field!r} not in result")
                return
            fn = OPS.get(op)
            if not fn(row[field], expected):
                _emit("fail", f"{field}={row[field]!r} failed {op} {expected!r}")
                return

    _emit("pass", f"all {len(rows)} row(s) matched assertions")


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
