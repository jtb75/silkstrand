"""Generic control evaluator for declarative YAML check definitions.

The evaluator understands a small set of check primitives; anything that
doesn't fit becomes a `custom` control pointing at a Python callable.
Nothing here is MSSQL-specific — the DB connection is passed in from the
bundle entrypoint so the same evaluator can back PostgreSQL / MySQL
bundles later.
"""

from __future__ import annotations

import importlib
import re
from pathlib import Path
from typing import Any, Callable, Iterable

import yaml


# ---------------------------------------------------------------------------
# Result constants
# ---------------------------------------------------------------------------

PASS = "PASS"
FAIL = "FAIL"
ERROR = "ERROR"
NOT_APPLICABLE = "NOT_APPLICABLE"


# ---------------------------------------------------------------------------
# Assertion operators
# ---------------------------------------------------------------------------

def _coerce(x):
    # Normalize numeric strings so "0" == 0 etc. Callers can always force
    # string semantics with `op: pattern_match`.
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


OPS: dict[str, Callable[[Any, Any], bool]] = {
    "equals":                 lambda a, b: _coerce(a) == _coerce(b),
    "not_equals":             lambda a, b: _coerce(a) != _coerce(b),
    "greater_than":           lambda a, b: _coerce(a) >  _coerce(b),
    "greater_than_or_equal":  lambda a, b: _coerce(a) >= _coerce(b),
    "less_than":              lambda a, b: _coerce(a) <  _coerce(b),
    "less_than_or_equal":     lambda a, b: _coerce(a) <= _coerce(b),
    "in":                     lambda a, b: _coerce(a) in [_coerce(x) for x in b],
    "not_in":                 lambda a, b: _coerce(a) not in [_coerce(x) for x in b],
    "pattern_match":          lambda a, b: re.search(b, str(a)) is not None,
    "pattern_not_match":      lambda a, b: re.search(b, str(a)) is None,
}


# ---------------------------------------------------------------------------
# Row helpers
# ---------------------------------------------------------------------------

def _row_matches_all(row: dict, assertions: list[dict]) -> tuple[bool, str | None]:
    """Return (matched, first-failing-assertion-description)."""
    for a in assertions:
        field, op, expected = a["field"], a["op"], a.get("value")
        if field not in row:
            return False, f"field {field!r} not in result row"
        fn = OPS.get(op)
        if fn is None:
            return False, f"unknown operator {op!r}"
        if not fn(row[field], expected):
            return False, f"{field}={row[field]!r} failed {op} {expected!r}"
    return True, None


def _evaluate_rows(rows: list[dict], existence: str, aggregation: str,
                   assertions: list[dict]) -> tuple[str, str]:
    """Apply existence + aggregation + assertions. Returns (status, detail)."""
    n = len(rows)

    # Existence gate first — it can short-circuit before we look at any row.
    if existence == "no_rows":
        if n == 0:
            return PASS, "no rows returned (as required)"
        return FAIL, f"{n} row(s) returned; expected none"
    if existence == "exactly_one_row" and n != 1:
        return FAIL, f"expected exactly 1 row, got {n}"
    if existence in ("at_least_one_row", None) and n == 0:
        return FAIL, "query returned no rows"

    # Now check assertions against the rows we got.
    if not assertions:
        return PASS, f"{n} row(s); no assertions configured"

    if aggregation in (None, "all_rows_match"):
        for row in rows:
            ok, why = _row_matches_all(row, assertions)
            if not ok:
                return FAIL, why or "assertion failed"
        return PASS, f"all {n} row(s) matched assertions"

    if aggregation == "any_row_matches":
        for row in rows:
            ok, _ = _row_matches_all(row, assertions)
            if ok:
                return PASS, f"at least one of {n} row(s) matched"
        return FAIL, f"no row in {n} matched assertions"

    return ERROR, f"unknown aggregation {aggregation!r}"


# ---------------------------------------------------------------------------
# Check primitives
# ---------------------------------------------------------------------------

def _run_query(cursor, sql: str) -> list[dict]:
    """Execute sql and return rows as list of column-name-keyed dicts."""
    cursor.execute(sql)
    cols = [d[0] for d in cursor.description] if cursor.description else []
    return [dict(zip(cols, r)) for r in cursor.fetchall()]


def _use_db(cursor, db: str | None) -> None:
    """Switch database context when the primitive declares a scope.
    Only runs a USE statement for trusted engine system names and for the
    per-DB iterator (which feeds names from sys.databases)."""
    if db:
        # Identifiers are quoted; the iterator only passes names it read
        # from sys.databases, so the quoted form is safe.
        cursor.execute(f"USE [{db}];")


def _iter_user_databases(cursor) -> list[str]:
    """Names of online user databases, excluding the four system DBs."""
    rows = _run_query(
        cursor,
        "SELECT name FROM sys.databases "
        "WHERE database_id > 4 AND state = 0 "  # online only
        "AND name NOT IN ('master','tempdb','model','msdb','rdsadmin');",
    )
    return [r["name"] for r in rows]


def _eval_sql_configuration(cursor, check: dict) -> tuple[str, str]:
    rows = _run_query(cursor, check["query"])
    return _evaluate_rows(
        rows,
        check.get("existence", "at_least_one_row"),
        check.get("aggregation", "all_rows_match"),
        check.get("assertions", []),
    )


def _eval_sql_scalar(cursor, check: dict) -> tuple[str, str]:
    """Query returning a single value; compare against assertions on a
    synthetic row with field name 'value'."""
    rows = _run_query(cursor, check["query"])
    if not rows:
        return FAIL, "query returned no rows"
    # Normalize first column to 'value' so YAML can say field: value
    first_col = next(iter(rows[0]))
    normalized = [{"value": r[first_col], **r} for r in rows]
    return _evaluate_rows(normalized,
                          check.get("existence", "at_least_one_row"),
                          "all_rows_match",
                          check.get("assertions", []))


def _eval_sql_no_rows_match(cursor, check: dict) -> tuple[str, str]:
    """PASS when the query returns zero rows (or zero rows match assertions).

    Useful for "ensure no orphan users", "ensure no unexpected superusers",
    etc. — expressible as "this query should return nothing."
    """
    _use_db(cursor, check.get("database"))
    rows = _run_query(cursor, check["query"])
    return _evaluate_rows(rows, "no_rows", "all_rows_match",
                          check.get("assertions", []))


def _eval_sql_no_rows_match_per_database(cursor, check: dict) -> tuple[str, str]:
    """Run the check query in every user database; PASS iff no database
    returns any rows. Used for CIS controls that say 'run this query in
    each database' and expect zero violations cluster-wide."""
    dbs = _iter_user_databases(cursor)
    if not dbs:
        return PASS, "no user databases present"

    offenders: list[str] = []
    for db in dbs:
        try:
            _use_db(cursor, db)
            rows = _run_query(cursor, check["query"])
        except Exception as exc:  # noqa: BLE001
            return ERROR, f"query failed in database {db!r}: {exc}"
        if rows:
            offenders.append(f"{db} ({len(rows)})")
    if offenders:
        return FAIL, "row(s) returned in: " + ", ".join(offenders)
    return PASS, f"clean across {len(dbs)} user database(s)"


PRIMITIVES = {
    "sql_configuration":                 _eval_sql_configuration,
    "sql_scalar":                        _eval_sql_scalar,
    "sql_no_rows_match":                 _eval_sql_no_rows_match,
    "sql_no_rows_match_per_database":    _eval_sql_no_rows_match_per_database,
}


# ---------------------------------------------------------------------------
# Control evaluation
# ---------------------------------------------------------------------------

def _id_key(control: dict):
    """Sort key that orders '2.2' before '2.11' (semantic, not lexical)."""
    parts = str(control.get("id", "")).split(".")
    out = []
    for p in parts:
        try:
            out.append((0, int(p)))
        except ValueError:
            out.append((1, p))
    return out


def _load_controls(controls_dir: Path) -> list[dict]:
    out = []
    for p in controls_dir.glob("*.yaml"):
        # Skip macOS AppleDouble companion files (._*.yaml). macOS `tar`
        # writes these alongside real files when the source has any
        # extended attribute; the binary metadata bombs the YAML loader.
        if p.name.startswith("._"):
            continue
        # Force UTF-8. launchd-spawned agents on macOS inherit no locale,
        # which flips open()'s default encoding away from UTF-8 and bombs
        # on em-dashes in rationale/remediation text.
        with p.open(encoding="utf-8") as f:
            doc = yaml.safe_load(f)
            doc["_source_file"] = p.name
            out.append(doc)
    out.sort(key=_id_key)
    return out


def _eval_precondition(cursor, pre: dict) -> tuple[bool, str | None]:
    """Returns (triggered, reason). Triggered means the control should be
    marked NOT_APPLICABLE (with the supplied reason)."""
    if not pre:
        return False, None
    fn = PRIMITIVES.get(pre["type"])
    if fn is None:
        return False, None
    status, _ = fn(cursor, pre)
    # Precondition is "triggered" when the inner check passes — that means
    # the state described by `if_matches` is true.
    if status == PASS:
        return True, (pre.get("if_matches") or {}).get("reason")
    return False, None


def _run_single(cursor, control: dict, custom_module) -> dict:
    check = control.get("check") or {}
    ctype = check.get("type")

    # Preconditions first.
    pre = check.get("precondition")
    try:
        triggered, reason = _eval_precondition(cursor, pre)
        if triggered:
            return _result(control, NOT_APPLICABLE,
                           reason or "precondition not met",
                           expected="N/A")
    except Exception as exc:  # noqa: BLE001
        return _result(control, ERROR, f"precondition error: {exc}")

    # Main check.
    try:
        if ctype == "custom":
            if custom_module is None:
                return _result(control, ERROR,
                               "custom check requested but no custom.py module loaded")
            fn_name = check["function"]
            fn = getattr(custom_module, fn_name, None)
            if fn is None:
                return _result(control, ERROR, f"custom function {fn_name!r} not found")
            status, detail = fn(cursor, check)
        else:
            fn = PRIMITIVES.get(ctype)
            if fn is None:
                return _result(control, ERROR, f"unknown check type {ctype!r}")
            status, detail = fn(cursor, check)
        return _result(control, status, detail)
    except Exception as exc:  # noqa: BLE001
        return _result(control, ERROR, f"check raised: {exc}")


def _result(control: dict, status: str, detail: str,
            expected: str | None = None) -> dict:
    return {
        "id": control["id"],
        "title": control["title"],
        "description": (control.get("description") or "").strip(),
        "status": status,
        "severity": control.get("severity", "MEDIUM"),
        "evidence": {
            "current_value": detail,
            "expected_value": expected if expected is not None
                              else _describe_expected(control),
            "source": (control.get("check") or {}).get("query", "N/A").strip().split("\n")[0][:200],
        },
        "remediation": (control.get("remediation") or "").strip(),
        "profile": control.get("profile", []),
        "references": control.get("references", []),
        "framework_mappings": control.get("framework_mappings", {}),
    }


def _describe_expected(control: dict) -> str:
    check = control.get("check") or {}
    parts = []
    for a in check.get("assertions") or []:
        parts.append(f"{a.get('field')} {a.get('op')} {a.get('value')!r}")
    return "; ".join(parts) if parts else (control.get("default_value") or "")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def evaluate_all(cursor, controls_dir: Path,
                 custom_module_name: str | None = None) -> list[dict]:
    custom_module = None
    if custom_module_name:
        try:
            custom_module = importlib.import_module(custom_module_name)
        except ImportError:
            custom_module = None

    controls = _load_controls(controls_dir)
    return [_run_single(cursor, c, custom_module) for c in controls]
