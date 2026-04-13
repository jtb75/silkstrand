# Authoring Compliance Bundles

How to take a CIS benchmark PDF (or equivalent prescriptive source) and turn it
into a SilkStrand compliance bundle that runs on the agent. Reference for both
humans and LLM subagents.

## Bundle anatomy

```
bundles/<benchmark-id>/
  manifest.yaml               # name, version, vendor_dir, entrypoint
  content/
    checks.py                 # entry point: connects to target, drives evaluator
    evaluator.py              # generic control evaluator (reusable across bundles)
    controls/*.yaml           # one file per control
    vendor/                   # pure-Python dependencies
```

One YAML file per control. The agent runs one bundle against one target and
emits a `silkstrand-v1` results JSON.

## Content licensing

Our organization holds **CIS SecureSuite Membership** and **CIS Benchmarks
Certified 3rd Party Tooling** licensing, which authorizes embedding CIS
benchmark content (including titles, rationale text, audit procedures,
remediation SQL, references) in SilkStrand.

**Transcribe faithfully.** Do not paraphrase security-meaningful content
(description, rationale, impact, audit, remediation, default_value,
references) — copy from the PDF. Field ordering in the YAML should follow
the existing files.

Non-CIS content (internal comments, structural YAML) may be freely written.

## Control YAML schema

```yaml
id: "2.1"                                      # CIS control number (string)
title: "Ensure 'Ad Hoc Distributed Queries'…"  # from the PDF heading (minus "(Automated)")
section: "Surface Area Reduction"              # CIS section name
assessment: automated                          # automated | manual
profile:                                       # list of CIS profiles from the PDF
  - "Level 1 - Database Engine"
  - "Level 1 - AWS RDS"
severity: MEDIUM                               # HIGH | MEDIUM | LOW — author judgment from impact

description: >                                 # transcribe from "Description:" in the PDF
  …
rationale: >                                   # transcribe from "Rationale:"
  …
impact: >                                      # transcribe from "Impact:" if present
  …

check:                                         # the only field the evaluator runs
  type: <primitive>
  …primitive-specific fields

remediation: |                                 # transcribe from "Remediation:" (SQL + prose)
  …
default_value: "0 (disabled)"                  # from "Default Value:"

references:                                    # from "References:" — URLs only
  - https://learn.microsoft.com/…

framework_mappings:                            # from the "CIS Controls:" table at the bottom
  cis_controls_v8:
    - id: "4.4"
      title: "Implement and Manage a Firewall on Servers"
  cis_controls_v7:
    - id: "9.2"
      title: "Ensure Only Approved Ports, Protocols and Services Are Running"
```

## Check primitives

Five primitives. Do **not** invent new ones without an evaluator change — propose
the addition first, implement and test, then use.

### `sql_configuration`

Run SQL, assert each returned row's fields match the expected values.

```yaml
check:
  type: sql_configuration
  query: |
    SELECT name, CAST(value AS int) AS value_configured, …
    FROM sys.configurations WHERE name = 'Ad Hoc Distributed Queries';
  existence: at_least_one_row       # at_least_one_row | exactly_one_row | no_rows
  aggregation: all_rows_match       # all_rows_match | any_row_matches
  assertions:
    - field: value_configured
      op: equals
      value: 0
```

**Use when** CIS's audit reads one or more per-object rows from a system view
and says "the column must equal X" (e.g. `sp_configure` checks, `sys.databases`
flag checks).

### `sql_scalar`

Run SQL returning a single value, assert on it via a synthetic `value` field.

```yaml
check:
  type: sql_scalar
  query: "SELECT SERVERPROPERTY('IsIntegratedSecurityOnly') AS login_mode;"
  assertions:
    - field: value
      op: equals
      value: 1
```

**Use when** CIS says "a value of X indicates compliance" — counts, boolean
properties, version numbers, single-row single-column outputs.

### `sql_no_rows_match`

PASS iff the query returns zero rows.

```yaml
check:
  type: sql_no_rows_match
  database: msdb                   # optional: runs USE [msdb] before the query
  query: |
    SELECT name FROM sys.databases WHERE is_trustworthy_on = 1 AND name != 'msdb';
```

**Use when** CIS says "this query should return no rows" — orphan users,
unexpected superusers, policy violators. Invert the audit query if necessary
so violations surface as rows.

### `sql_no_rows_match_per_database`

Iterates every online user database (system DBs excluded automatically). PASS
iff zero rows anywhere.

```yaml
check:
  type: sql_no_rows_match_per_database
  query: |
    SELECT DB_NAME() AS DatabaseName, …
    FROM sys.database_permissions WHERE …;
```

**Use when** CIS says "run this in each database" and expects zero violations
cluster-wide (guest CONNECT, orphan users, contained-DB SQL auth, per-DB
encryption checks).

### `custom`

Escape hatch: name a Python function in the bundle's `custom.py`.

```yaml
check:
  type: custom
  function: check_pg_hba_no_trust
```

**Use only when** no other primitive fits — e.g. multi-step logic, per-row
conditional remediation triggers, or things that need host-side parsing.

## Preconditions

Any primitive can be gated by a precondition that short-circuits to
`NOT_APPLICABLE` when some state holds:

```yaml
check:
  precondition:
    type: sql_scalar
    query: "SELECT CAST(SERVERPROPERTY('IsClustered') AS int) AS value;"
    if_matches:
      status: NOT_APPLICABLE
      reason: "clustered installation — DAC must remain reachable remotely"
    assertions:
      - field: value
        op: equals
        value: 1
  type: sql_configuration
  …
```

**Use when** CIS explicitly states "this recommendation is N/A if …" (e.g.
CIS 2.2 is N/A when `clr strict security = 1`; CIS 2.7 is N/A on clusters).

## Assertion operators

`equals`, `not_equals`, `greater_than`, `greater_than_or_equal`, `less_than`,
`less_than_or_equal`, `in`, `not_in`, `pattern_match`, `pattern_not_match`.

Numeric values are coerced before comparison so `0` and `"0"` match under
`equals`. Force string semantics with `pattern_match`.

## What to skip

Not every CIS recommendation can be automated via a database query. Skip a
control when its audit procedure requires:

- **Filesystem access** (file permissions on host, inspecting config files on disk)
- **OS-level introspection** (Windows service account membership, Linux package versions)
- **GUI verification** (SSMS checkbox state, Configuration Manager dialogs)
- **Human judgment** ("review the list and confirm each account is still in use")

These are legitimately Manual. Do not force-fit them into a SQL primitive —
the check will lie about compliance.

## Authoring a new section

1. **Read the PDF section.** Identify every `(Automated)` control. Skip
   `(Manual)` controls unless the audit procedure is actually pure SQL.
2. **Pick a primitive** per control using the guide above. If nothing fits,
   flag it for a primitive extension or escape to `custom` — don't invent a
   new primitive field.
3. **Author YAML.** Match existing files in the bundle for field ordering.
   One file per control, named `<id>-<short-slug>.yaml`.
4. **Transcribe content faithfully** per the licensing note.
5. **Verify.** If a live target is available, run `content/checks.py` with
   test config + credentials. Otherwise ensure YAML parses and control loads.

## Naming conventions

- Bundle directory: `cis-<tech>-<major_version>` (e.g. `cis-mssql-2022`)
- Bundle ID in manifest: same as directory name
- Control file: `<id>-<kebab-case-slug>.yaml` (e.g. `2.1-ad-hoc-distributed-queries.yaml`)
- Section name: match CIS PDF exactly (e.g. `Surface Area Reduction`, not `surface_area_reduction`)

## Driver choice

Drivers must be pure-Python, vendored under `content/vendor/` with their
`*.dist-info` directories (some packages do metadata lookups at import).
No compiled extensions — the agent runs on heterogeneous hosts.

| Target | Driver | Notes |
|---|---|---|
| PostgreSQL | `pg8000` + `scramp`, `asn1crypto`, `dateutil`, `six` | Already vendored in `cis-postgresql-16` |
| SQL Server | `pytds` (package `python-tds`) | Already vendored in `cis-mssql-2022` |
| MongoDB | `pymongo` (check if pure-Python on current version) | Not yet vendored |
| MySQL / MariaDB | `pymysql` | Not yet vendored |
| Redis | `redis-py` | Not yet vendored |
