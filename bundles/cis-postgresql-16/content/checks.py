#!/usr/bin/env python3
"""CIS PostgreSQL 16 Benchmark — SilkStrand compliance bundle.

Reads target connection params from SILKSTRAND_TARGET_CONFIG and
credentials from SILKSTRAND_CREDENTIALS (as written by the agent runner),
connects via pg8000, iterates every control YAML in controls/, and emits
a silkstrand-v1 results JSON document on stdout.
"""

import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

# Prepend vendored pure-Python deps (pg8000, scramp, yaml, …) so the host
# only needs python3.
_BUNDLE_DIR = Path(__file__).resolve().parent
_VENDOR_DIR = _BUNDLE_DIR / "vendor"
if str(_VENDOR_DIR) not in sys.path:
    sys.path.insert(0, str(_VENDOR_DIR))

import pg8000.dbapi  # noqa: E402

import evaluator  # noqa: E402


BUNDLE_NAME = "cis-postgresql-16"
BUNDLE_VERSION = "1.0.0"
CONTROLS_DIR = _BUNDLE_DIR / "controls"


def _log(msg: str) -> None:
    print(f"[checks] {msg}", file=sys.stderr)


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _read_json(path: str) -> dict:
    with open(path) as f:
        return json.load(f)


def _build_summary(controls: list[dict]) -> dict:
    s = {"total": len(controls), "pass": 0, "fail": 0, "error": 0, "not_applicable": 0}
    m = {"PASS": "pass", "FAIL": "fail", "ERROR": "error", "NOT_APPLICABLE": "not_applicable"}
    for c in controls:
        s[m.get(c["status"], "error")] += 1
    return s


def _error_result_all_controls(error_msg: str, target_identifier: str,
                               started: str) -> dict:
    """Build a silkstrand-v1 doc where every control reports ERROR — used
    when we can't even establish the DB connection."""
    import yaml  # local to avoid import unless needed
    controls = []
    for path in sorted(CONTROLS_DIR.glob("*.yaml")):
        with path.open() as f:
            c = yaml.safe_load(f)
        controls.append({
            "id": c.get("id", "?"),
            "title": c.get("title", path.stem),
            "description": (c.get("description") or "").strip(),
            "status": "ERROR",
            "severity": c.get("severity", "MEDIUM"),
            "evidence": {
                "current_value": f"error: {error_msg}",
                "expected_value": "N/A",
                "source": "connection",
            },
            "remediation": (c.get("remediation") or "").strip(),
            "profile": c.get("profile", []),
            "references": c.get("references", []),
            "framework_mappings": c.get("framework_mappings", {}),
        })
    return {
        "schema_version": "1",
        "bundle": {"name": BUNDLE_NAME, "version": BUNDLE_VERSION},
        "target": {"type": "database", "identifier": target_identifier},
        "started_at": started,
        "completed_at": _now_iso(),
        "status": "completed",
        "summary": _build_summary(controls),
        "controls": controls,
    }


def main() -> None:
    started = _now_iso()

    config_path = os.environ.get("SILKSTRAND_TARGET_CONFIG")
    if not config_path:
        _log("SILKSTRAND_TARGET_CONFIG not set")
        sys.exit(1)
    try:
        config = _read_json(config_path)
    except (OSError, json.JSONDecodeError) as exc:
        _log(f"reading target config: {exc}")
        sys.exit(1)

    creds = {}
    creds_path = os.environ.get("SILKSTRAND_CREDENTIALS")
    if creds_path:
        try:
            creds = _read_json(creds_path)
        except (OSError, json.JSONDecodeError) as exc:
            _log(f"reading credentials: {exc}")
            sys.exit(1)

    host = config.get("host", "localhost")
    port = int(config.get("port", 5432))
    database = config.get("database", "postgres")
    username = creds.get("username") or config.get("username", "postgres")
    password = creds.get("password") or config.get("password", "")
    sslmode = config.get("sslmode", "prefer")

    # pg8000 takes ssl_context (truthy / falsy / SSLContext), not "sslmode".
    # Map common psycopg2-style values: disable -> no SSL; everything else -> True.
    ssl_context = None if sslmode in ("disable", "allow", "") else True

    target_identifier = f"{host}:{port}/{database}"
    _log(f"connecting to {target_identifier} as {username}")

    try:
        conn = pg8000.dbapi.connect(
            host=host,
            port=port,
            database=database,
            user=username,
            password=password,
            ssl_context=ssl_context,
            timeout=10,
        )
        conn.autocommit = True
    except Exception as exc:  # noqa: BLE001
        _log(f"connection failed: {exc}")
        print(json.dumps(_error_result_all_controls(str(exc), target_identifier, started),
                         indent=2))
        sys.exit(0)

    try:
        cursor = conn.cursor()
        controls = evaluator.evaluate_all(cursor, CONTROLS_DIR)
    finally:
        try: conn.close()
        except Exception: pass

    result = {
        "schema_version": "1",
        "bundle": {"name": BUNDLE_NAME, "version": BUNDLE_VERSION},
        "target": {"type": "database", "identifier": target_identifier},
        "started_at": started,
        "completed_at": _now_iso(),
        "status": "completed",
        "summary": _build_summary(controls),
        "controls": controls,
    }
    print(json.dumps(result, indent=2))
    sys.exit(0)


if __name__ == "__main__":
    main()
