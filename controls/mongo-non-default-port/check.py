#!/usr/bin/env python3
"""Control: mongo-non-default-port — Ensure that MongoDB uses a non-default port"""

import json
import os
import re
import sys

CONTROL_ID = "mongo-non-default-port"
TITLE = "Ensure that MongoDB uses a non-default port"
SEVERITY = "low"
DB_NAME = "admin"
COMMAND = {'getCmdLineOpts': 1}

ASSERTIONS = [
    ("parsed.net.port", "not_equals", 27017),
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


def _get_path(doc, path):
    cur = doc
    for seg in path.split("."):
        if not isinstance(cur, dict) or seg not in cur:
            return None
        cur = cur[seg]
    return cur


OPS = {
    "equals": lambda a, b: _coerce(a) == _coerce(b),
    "not_equals": lambda a, b: _coerce(a) != _coerce(b),
    "greater_than_or_equal": lambda a, b: _coerce(a) >= _coerce(b),
    "contains": lambda a, b: b in (a or []) if isinstance(a, (list, tuple, str)) else False,
    "exists": lambda a, b: (a is not None) == bool(b),
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
    port = int(config.get("port", 27017))
    auth_source = config.get("auth_source", "admin")
    username = creds.get("username") or config.get("username")
    password = creds.get("password") or config.get("password")
    tls = bool(config.get("tls", False))

    sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "bundles", "cis-mongodb-8", "content", "vendor"))
    from pymongo import MongoClient  # noqa: E402

    kwargs = {
        "host": host, "port": port,
        "serverSelectionTimeoutMS": 10000,
        "tls": tls,
    }
    if username:
        kwargs["username"] = username
        kwargs["password"] = password
        kwargs["authSource"] = auth_source

    try:
        client = MongoClient(**kwargs)
        client.admin.command("ping")
    except Exception as exc:
        _emit("error", f"connection failed: {exc}")
        return

    try:
        res = client[DB_NAME].command(COMMAND)
    except Exception as exc:
        _emit("error", f"command failed: {exc}")
        return
    finally:
        try:
            client.close()
        except Exception:
            pass

    for field, op, expected in ASSERTIONS:
        actual = _get_path(res, field)
        fn = OPS.get(op)
        if fn is None:
            _emit("error", f"unknown operator {op!r}")
            return
        if not fn(actual, expected):
            _emit("fail", f"{field}={actual!r} failed {op} {expected!r}")
            return

    _emit("pass", f"all {len(ASSERTIONS)} assertion(s) matched")


def _emit(status, detail):
    result = {
        "control_id": CONTROL_ID,
        "status": status,
        "severity": SEVERITY,
        "title": TITLE,
        "evidence": {"detail": detail},
        "remediation": "Refer to CIS MongoDB 8 Benchmark section for remediation steps.",
    }
    print(json.dumps(result))


if __name__ == "__main__":
    main()
