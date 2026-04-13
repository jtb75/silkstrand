#!/usr/bin/env python3
"""CIS PostgreSQL 16 Benchmark — SilkStrand compliance checks.

Reads connection parameters from the JSON file pointed to by
SILKSTRAND_TARGET_CONFIG, runs eight CIS benchmark controls, and
emits a silkstrand-v1 results JSON document on stdout.

Exit 0 on success (even if checks FAIL — that is a valid result).
Exit 1 only for unrecoverable script-level errors.
"""

import json
import os
import sys
from datetime import datetime, timezone

# pg8000 (pure-Python PostgreSQL driver) is vendored under content/vendor.
# The agent runner prepends this directory to PYTHONPATH automatically when
# the manifest declares vendor_dir, but we also self-bootstrap so the bundle
# can be executed directly (e.g. local dev, ad-hoc testing).
_BUNDLE_DIR = os.path.dirname(os.path.abspath(__file__))
_VENDOR_DIR = os.path.join(_BUNDLE_DIR, "vendor")
if _VENDOR_DIR not in sys.path:
    sys.path.insert(0, _VENDOR_DIR)

import pg8000.dbapi  # noqa: E402
from pg8000.exceptions import DatabaseError  # noqa: E402


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _log(msg: str) -> None:
    """Write diagnostic output to stderr so stdout stays clean JSON."""
    print(f"[checks] {msg}", file=sys.stderr)


def _now_iso() -> str:
    return datetime.now(timezone.utc).isoformat()


def _query_one(cur, sql: str):
    """Execute a single-row query and return the first column value."""
    cur.execute(sql)
    row = cur.fetchone()
    return row[0] if row else None


def _show_setting(cur, name: str) -> str:
    """Return the value of a PostgreSQL GUC parameter."""
    return _query_one(cur, f"SHOW {name}")


# ---------------------------------------------------------------------------
# Individual checks — each returns a control dict
# ---------------------------------------------------------------------------

def check_1_2_log_connections(cur) -> dict:
    """1.2 — Ensure log_connections is enabled."""
    try:
        val = _show_setting(cur, "log_connections")
        return {
            "id": "1.2",
            "title": "Ensure log_connections is enabled",
            "description": (
                "Enabling log_connections causes each attempted connection to "
                "the server to be logged, along with successful completion of "
                "client authentication."
            ),
            "status": "PASS" if val == "on" else "FAIL",
            "severity": "MEDIUM",
            "evidence": {
                "current_value": val,
                "expected_value": "on",
                "source": "SHOW log_connections",
            },
            "remediation": (
                "Set log_connections = on in postgresql.conf and reload the server."
            ),
        }
    except Exception as exc:
        return _error_control("1.2", "Ensure log_connections is enabled", str(exc))


def check_1_3_log_disconnections(cur) -> dict:
    """1.3 — Ensure log_disconnections is enabled."""
    try:
        val = _show_setting(cur, "log_disconnections")
        return {
            "id": "1.3",
            "title": "Ensure log_disconnections is enabled",
            "description": (
                "Enabling log_disconnections logs the end of each session, "
                "including the duration."
            ),
            "status": "PASS" if val == "on" else "FAIL",
            "severity": "MEDIUM",
            "evidence": {
                "current_value": val,
                "expected_value": "on",
                "source": "SHOW log_disconnections",
            },
            "remediation": (
                "Set log_disconnections = on in postgresql.conf and reload the server."
            ),
        }
    except Exception as exc:
        return _error_control("1.3", "Ensure log_disconnections is enabled", str(exc))


def check_2_1_ssl(cur) -> dict:
    """2.1 — Ensure SSL is enabled."""
    try:
        val = _show_setting(cur, "ssl")
        return {
            "id": "2.1",
            "title": "Ensure SSL is enabled",
            "description": (
                "SSL provides encryption of data in transit, preventing "
                "eavesdropping on database connections."
            ),
            "status": "PASS" if val == "on" else "FAIL",
            "severity": "HIGH",
            "evidence": {
                "current_value": val,
                "expected_value": "on",
                "source": "SHOW ssl",
            },
            "remediation": (
                "Set ssl = on in postgresql.conf, configure ssl_cert_file and "
                "ssl_key_file, then restart the server."
            ),
        }
    except Exception as exc:
        return _error_control("2.1", "Ensure SSL is enabled", str(exc))


def check_3_1_password_encryption(cur) -> dict:
    """3.1 — Ensure password_encryption is set to scram-sha-256."""
    try:
        val = _show_setting(cur, "password_encryption")
        return {
            "id": "3.1",
            "title": "Ensure password_encryption is set to scram-sha-256",
            "description": (
                "SCRAM-SHA-256 provides stronger password hashing than the "
                "legacy MD5 method."
            ),
            "status": "PASS" if val == "scram-sha-256" else "FAIL",
            "severity": "HIGH",
            "evidence": {
                "current_value": val,
                "expected_value": "scram-sha-256",
                "source": "SHOW password_encryption",
            },
            "remediation": (
                "Set password_encryption = 'scram-sha-256' in postgresql.conf "
                "and reload the server. Re-set user passwords so they are "
                "stored with the new algorithm."
            ),
        }
    except Exception as exc:
        return _error_control(
            "3.1", "Ensure password_encryption is set to scram-sha-256", str(exc)
        )


def check_4_1_pg_hba_no_trust(cur) -> dict:
    """4.1 — Ensure pg_hba.conf does not allow trust authentication for host connections."""
    title = "Ensure pg_hba.conf does not use trust authentication for host connections"
    try:
        # pg_hba_file_rules is available in PG 15+
        cur.execute(
            "SELECT 1 FROM pg_catalog.pg_class "
            "WHERE relname = 'pg_hba_file_rules' AND relkind = 'v'"
        )
        view_exists = cur.fetchone() is not None

        if not view_exists:
            return {
                "id": "4.1",
                "title": title,
                "description": (
                    "The trust authentication method allows anyone who can "
                    "connect to the server to access the database without a "
                    "password."
                ),
                "status": "NOT_APPLICABLE",
                "severity": "HIGH",
                "evidence": {
                    "current_value": "N/A",
                    "expected_value": "no trust entries for host connections",
                    "source": "pg_hba_file_rules view not available (requires PG 15+)",
                },
                "remediation": (
                    "Manually inspect pg_hba.conf and remove any trust entries "
                    "for host connections."
                ),
            }

        cur.execute(
            "SELECT line_number, type, auth_method "
            "FROM pg_hba_file_rules "
            "WHERE type IN ('host', 'hostssl', 'hostnossl', 'hostgssenc', 'hostnogssenc') "
            "AND auth_method = 'trust'"
        )
        trust_rows = cur.fetchall()

        if trust_rows:
            details = "; ".join(
                f"line {r[0]}: type={r[1]} auth={r[2]}" for r in trust_rows
            )
            return {
                "id": "4.1",
                "title": title,
                "description": (
                    "The trust authentication method allows anyone who can "
                    "connect to the server to access the database without a "
                    "password."
                ),
                "status": "FAIL",
                "severity": "HIGH",
                "evidence": {
                    "current_value": details,
                    "expected_value": "no trust entries for host connections",
                    "source": "pg_hba_file_rules",
                },
                "remediation": (
                    "Edit pg_hba.conf and change the authentication method from "
                    "'trust' to 'scram-sha-256' for all host entries, then reload."
                ),
            }

        return {
            "id": "4.1",
            "title": title,
            "description": (
                "The trust authentication method allows anyone who can "
                "connect to the server to access the database without a "
                "password."
            ),
            "status": "PASS",
            "severity": "HIGH",
            "evidence": {
                "current_value": "no trust entries found",
                "expected_value": "no trust entries for host connections",
                "source": "pg_hba_file_rules",
            },
            "remediation": "No action required.",
        }

    except DatabaseError as exc:
        # pg8000 doesn't expose a typed InsufficientPrivilege; the SQLSTATE
        # for it is 42501. Fall through to a generic error otherwise.
        sqlstate = ""
        if exc.args and isinstance(exc.args[0], dict):
            sqlstate = exc.args[0].get("C", "")
        if sqlstate != "42501":
            return _error_control("4.1", title, str(exc))
        return {
            "id": "4.1",
            "title": title,
            "description": (
                "The trust authentication method allows anyone who can "
                "connect to the server to access the database without a "
                "password."
            ),
            "status": "ERROR",
            "severity": "HIGH",
            "evidence": {
                "current_value": "insufficient privileges to read pg_hba_file_rules",
                "expected_value": "no trust entries for host connections",
                "source": "pg_hba_file_rules",
            },
            "remediation": (
                "Grant the scanning role access to pg_hba_file_rules, or "
                "manually inspect pg_hba.conf."
            ),
        }
    except Exception as exc:
        return _error_control("4.1", title, str(exc))


def check_5_1_log_statement(cur) -> dict:
    """5.1 — Ensure log_statement is set to 'all' or 'ddl'."""
    try:
        val = _show_setting(cur, "log_statement")
        acceptable = ("all", "ddl")
        return {
            "id": "5.1",
            "title": "Ensure log_statement is set to 'ddl' or 'all'",
            "description": (
                "Setting log_statement ensures that SQL statements are logged, "
                "aiding in auditing and forensic analysis."
            ),
            "status": "PASS" if val in acceptable else "FAIL",
            "severity": "MEDIUM",
            "evidence": {
                "current_value": val,
                "expected_value": "ddl or all",
                "source": "SHOW log_statement",
            },
            "remediation": (
                "Set log_statement = 'ddl' (or 'all') in postgresql.conf and "
                "reload the server."
            ),
        }
    except Exception as exc:
        return _error_control(
            "5.1", "Ensure log_statement is set to 'ddl' or 'all'", str(exc)
        )


def check_6_1_pgaudit(cur) -> dict:
    """6.1 — Ensure pgAudit extension is available."""
    try:
        cur.execute(
            "SELECT name FROM pg_available_extensions WHERE name = 'pgaudit'"
        )
        row = cur.fetchone()
        available = row is not None
        return {
            "id": "6.1",
            "title": "Ensure pgAudit extension is available",
            "description": (
                "pgAudit provides detailed session and object audit logging "
                "via the standard PostgreSQL logging facility."
            ),
            "status": "PASS" if available else "FAIL",
            "severity": "MEDIUM",
            "evidence": {
                "current_value": "available" if available else "not available",
                "expected_value": "available",
                "source": "pg_available_extensions",
            },
            "remediation": (
                "Install the pgAudit extension package for your PostgreSQL "
                "distribution and add 'pgaudit' to shared_preload_libraries."
            ),
        }
    except Exception as exc:
        return _error_control("6.1", "Ensure pgAudit extension is available", str(exc))


def check_7_1_superuser_roles(cur) -> dict:
    """7.1 — Ensure no unexpected superuser roles exist."""
    try:
        cur.execute(
            "SELECT rolname FROM pg_roles WHERE rolsuper = true ORDER BY rolname"
        )
        superusers = [r[0] for r in cur.fetchall()]
        # Only 'postgres' (the default superuser) is expected
        unexpected = [u for u in superusers if u != "postgres"]
        if unexpected:
            status = "FAIL"
            current = f"superusers: {', '.join(superusers)}"
        else:
            status = "PASS"
            current = f"superusers: {', '.join(superusers)}"

        return {
            "id": "7.1",
            "title": "Ensure no unexpected superuser roles exist",
            "description": (
                "Superuser roles bypass all permission checks. Only the "
                "default 'postgres' role should have superuser privileges."
            ),
            "status": status,
            "severity": "HIGH",
            "evidence": {
                "current_value": current,
                "expected_value": "only 'postgres' should be a superuser",
                "source": "pg_roles WHERE rolsuper = true",
            },
            "remediation": (
                "Revoke superuser from unnecessary roles: "
                "ALTER ROLE <role> NOSUPERUSER;"
            ),
        }
    except Exception as exc:
        return _error_control(
            "7.1", "Ensure no unexpected superuser roles exist", str(exc)
        )


def _error_control(control_id: str, title: str, message: str) -> dict:
    """Return a control result with ERROR status."""
    return {
        "id": control_id,
        "title": title,
        "description": "",
        "status": "ERROR",
        "severity": "MEDIUM",
        "evidence": {
            "current_value": f"error: {message}",
            "expected_value": "N/A",
            "source": "N/A",
        },
        "remediation": "Investigate the error and re-run the check.",
    }


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

ALL_CHECKS = [
    check_1_2_log_connections,
    check_1_3_log_disconnections,
    check_2_1_ssl,
    check_3_1_password_encryption,
    check_4_1_pg_hba_no_trust,
    check_5_1_log_statement,
    check_6_1_pgaudit,
    check_7_1_superuser_roles,
]


def _build_error_result(target_identifier: str, error_msg: str, started: str) -> dict:
    """Build a result document where all controls are ERROR (e.g. connection failure)."""
    controls = []
    for check_fn in ALL_CHECKS:
        # Extract id and title from the docstring (format: "X.Y — Title")
        doc = check_fn.__doc__ or ""
        parts = doc.split(" — ", 1)
        cid = parts[0].strip() if parts else "?"
        title = parts[1].strip() if len(parts) > 1 else check_fn.__name__
        controls.append(_error_control(cid, title, error_msg))

    return {
        "schema_version": "1",
        "bundle": {"name": "cis-postgresql-16", "version": "1.0.2"},
        "target": {"type": "database", "identifier": target_identifier},
        "started_at": started,
        "completed_at": _now_iso(),
        "status": "completed",
        "summary": {
            "total": len(controls),
            "pass": 0,
            "fail": 0,
            "error": len(controls),
            "not_applicable": 0,
        },
        "controls": controls,
    }


def main() -> None:
    started = _now_iso()

    # --- Read target config ---
    config_path = os.environ.get("SILKSTRAND_TARGET_CONFIG")
    if not config_path:
        _log("SILKSTRAND_TARGET_CONFIG environment variable is not set")
        sys.exit(1)

    try:
        with open(config_path, "r") as f:
            config = json.load(f)
    except (OSError, json.JSONDecodeError) as exc:
        _log(f"Failed to read target config from {config_path}: {exc}")
        sys.exit(1)

    # Credentials are written to a separate file by the agent runner
    # (SILKSTRAND_CREDENTIALS); merge them into the connection params so
    # secret material never has to live in target_config.
    creds = {}
    creds_path = os.environ.get("SILKSTRAND_CREDENTIALS")
    if creds_path:
        try:
            with open(creds_path, "r") as f:
                creds = json.load(f)
        except (OSError, json.JSONDecodeError) as exc:
            _log(f"Failed to read credentials from {creds_path}: {exc}")
            sys.exit(1)

    host = config.get("host", "localhost")
    port = config.get("port", 5432)
    database = config.get("database", "postgres")
    username = creds.get("username") or config.get("username", "postgres")
    password = creds.get("password") or config.get("password", "")
    sslmode = config.get("sslmode", "prefer")

    target_identifier = f"{host}:{port}/{database}"
    _log(f"Connecting to {target_identifier} as {username}")

    # --- Connect ---
    # pg8000 takes ssl_context (truthy / falsy / SSLContext), not "sslmode".
    # Map common psycopg2-style values: disable -> no SSL; everything else -> True.
    if sslmode in ("disable", "allow", ""):
        ssl_context = None
    else:
        ssl_context = True

    try:
        conn = pg8000.dbapi.connect(
            host=host,
            port=int(port),
            database=database,
            user=username,
            password=password,
            ssl_context=ssl_context,
            timeout=10,
        )
        conn.autocommit = True
    except Exception as exc:
        _log(f"Connection failed: {exc}")
        result = _build_error_result(target_identifier, str(exc), started)
        print(json.dumps(result, indent=2))
        sys.exit(0)

    # --- Run checks ---
    controls = []
    cur = conn.cursor()
    try:
        for check_fn in ALL_CHECKS:
            _log(f"Running {check_fn.__name__}")
            controls.append(check_fn(cur))
    finally:
        cur.close()
        conn.close()

    # --- Compute summary ---
    summary = {"total": len(controls), "pass": 0, "fail": 0, "error": 0, "not_applicable": 0}
    status_map = {"PASS": "pass", "FAIL": "fail", "ERROR": "error", "NOT_APPLICABLE": "not_applicable"}
    for ctrl in controls:
        key = status_map.get(ctrl["status"], "error")
        summary[key] += 1

    result = {
        "schema_version": "1",
        "bundle": {"name": "cis-postgresql-16", "version": "1.0.2"},
        "target": {"type": "database", "identifier": target_identifier},
        "started_at": started,
        "completed_at": _now_iso(),
        "status": "completed",
        "summary": summary,
        "controls": controls,
    }

    print(json.dumps(result, indent=2))
    sys.exit(0)


if __name__ == "__main__":
    main()
