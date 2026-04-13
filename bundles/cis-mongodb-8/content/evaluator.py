"""Control evaluator for MongoDB bundles.

Mirrors the structure of the SQL evaluator (YAML loading, precondition
short-circuit, silkstrand-v1 output) but implements a MongoDB-specific
check primitive: run an admin command and assert on the returned
document. Assertions operate on dotted-path field access into the
response document.
"""

from __future__ import annotations

import importlib
import re
from pathlib import Path
from typing import Any, Callable

import yaml


PASS = "PASS"
FAIL = "FAIL"
ERROR = "ERROR"
NOT_APPLICABLE = "NOT_APPLICABLE"


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


OPS: dict[str, Callable[[Any, Any], bool]] = {
    "equals":                 lambda a, b: _coerce(a) == _coerce(b),
    "not_equals":             lambda a, b: _coerce(a) != _coerce(b),
    "greater_than":           lambda a, b: _coerce(a) >  _coerce(b),
    "greater_than_or_equal":  lambda a, b: _coerce(a) >= _coerce(b),
    "less_than":              lambda a, b: _coerce(a) <  _coerce(b),
    "less_than_or_equal":     lambda a, b: _coerce(a) <= _coerce(b),
    "in":                     lambda a, b: _coerce(a) in [_coerce(x) for x in b],
    "not_in":                 lambda a, b: _coerce(a) not in [_coerce(x) for x in b],
    "contains":               lambda a, b: b in (a or []) if isinstance(a, (list, tuple, str)) else False,
    "not_contains":           lambda a, b: b not in (a or []) if isinstance(a, (list, tuple, str)) else True,
    "pattern_match":          lambda a, b: re.search(b, str(a)) is not None,
    "pattern_not_match":      lambda a, b: re.search(b, str(a)) is None,
    "exists":                 lambda a, b: (a is not None) == bool(b),
}


def _get_path(doc: dict, path: str):
    """Dotted path lookup into a nested dict. Returns None if any
    segment is missing. 'auditLog.destination' → doc['auditLog']['destination']."""
    cur = doc
    for seg in path.split("."):
        if not isinstance(cur, dict) or seg not in cur:
            return None
        cur = cur[seg]
    return cur


# ---------------------------------------------------------------------------
# Primitive: mongodb_command
# ---------------------------------------------------------------------------

def _eval_mongodb_command(client, check: dict) -> tuple[str, str]:
    """Run a Mongo admin command and assert on the response document.

    YAML shape:
      type: mongodb_command
      database: admin              # optional; default 'admin'
      command:                     # BSON-style document
        getParameter: 1
        authenticationMechanisms: 1
      assertions:
        - field: authenticationMechanisms   # dotted path into response
          op: contains
          value: "SCRAM-SHA-256"
    """
    db_name = check.get("database", "admin")
    cmd = check["command"]
    try:
        res = client[db_name].command(cmd)
    except Exception as exc:  # noqa: BLE001
        return ERROR, f"command failed: {exc}"

    for a in check.get("assertions", []):
        field, op, expected = a["field"], a["op"], a.get("value")
        actual = _get_path(res, field)
        fn = OPS.get(op)
        if fn is None:
            return ERROR, f"unknown operator {op!r}"
        if not fn(actual, expected):
            return FAIL, f"{field}={actual!r} failed {op} {expected!r}"
    return PASS, f"all {len(check.get('assertions', []))} assertion(s) matched"


def _eval_mongodb_no_rows_match(client, check: dict) -> tuple[str, str]:
    """Run a find() on a collection; PASS iff no documents returned.

    YAML shape:
      type: mongodb_no_rows_match
      database: admin
      collection: system.users
      filter: { roles: { $elemMatch: { role: "root" } } }
    """
    db_name = check.get("database")
    coll = check["collection"]
    flt = check.get("filter", {})
    try:
        docs = list(client[db_name][coll].find(flt).limit(50))
    except Exception as exc:  # noqa: BLE001
        return ERROR, f"find failed: {exc}"
    if docs:
        return FAIL, f"{len(docs)} document(s) matched; expected none"
    return PASS, "no documents matched"


PRIMITIVES = {
    "mongodb_command":         _eval_mongodb_command,
    "mongodb_no_rows_match":   _eval_mongodb_no_rows_match,
}


# ---------------------------------------------------------------------------
# Control loading + evaluation (shared shape with sql evaluator)
# ---------------------------------------------------------------------------

def _id_key(control: dict):
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
        with p.open() as f:
            doc = yaml.safe_load(f)
            doc["_source_file"] = p.name
            out.append(doc)
    out.sort(key=_id_key)
    return out


def _eval_precondition(client, pre: dict) -> tuple[bool, str | None]:
    if not pre:
        return False, None
    fn = PRIMITIVES.get(pre["type"])
    if fn is None:
        return False, None
    status, _ = fn(client, pre)
    if status == PASS:
        return True, (pre.get("if_matches") or {}).get("reason")
    return False, None


def _run_single(client, control: dict, custom_module) -> dict:
    check = control.get("check") or {}
    ctype = check.get("type")

    pre = check.get("precondition")
    try:
        triggered, reason = _eval_precondition(client, pre)
        if triggered:
            return _result(control, NOT_APPLICABLE,
                           reason or "precondition not met", expected="N/A")
    except Exception as exc:  # noqa: BLE001
        return _result(control, ERROR, f"precondition error: {exc}")

    try:
        if ctype == "custom":
            if custom_module is None:
                return _result(control, ERROR,
                               "custom check requested but no custom.py module loaded")
            fn = getattr(custom_module, check["function"], None)
            if fn is None:
                return _result(control, ERROR, f"custom function {check['function']!r} not found")
            status, detail = fn(client, check)
        else:
            fn = PRIMITIVES.get(ctype)
            if fn is None:
                return _result(control, ERROR, f"unknown check type {ctype!r}")
            status, detail = fn(client, check)
        return _result(control, status, detail)
    except Exception as exc:  # noqa: BLE001
        return _result(control, ERROR, f"check raised: {exc}")


def _result(control: dict, status: str, detail: str,
            expected: str | None = None) -> dict:
    check = control.get("check") or {}
    source = ""
    if "command" in check:
        source = str(check["command"])[:200]
    elif "collection" in check:
        source = f"{check.get('database','?')}.{check['collection']}"
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
            "source": source,
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


def evaluate_all(client, controls_dir: Path,
                 custom_module_name: str | None = None) -> list[dict]:
    custom_module = None
    if custom_module_name:
        try:
            custom_module = importlib.import_module(custom_module_name)
        except ImportError:
            custom_module = None
    controls = _load_controls(controls_dir)
    return [_run_single(client, c, custom_module) for c in controls]
