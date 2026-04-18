# ADR 011: Collector + policy split — facts on the agent, evaluation on the server

**Status:** Proposed
**Date:** 2026-04-18
**Related:** [ADR 010](./010-hybrid-bundles.md) (hybrid bundles — controls become
collectors), [ADR 004](./004-credential-resolver.md) (credential resolution —
collectors need credentials), [ADR 007](./007-findings-scheduler.md) (findings —
evaluation output).

---

## Context

Today each compliance control is a monolithic Python script that:

1. Connects to the target database
2. Runs one or more queries
3. Evaluates the results against a hardcoded policy
4. Returns pass/fail + evidence

This bundles **data collection** (privileged, engine-specific, runs on the
customer network) with **policy evaluation** (declarative, auditable, should
be centrally managed). The consequences:

- Changing a policy threshold (e.g., "accept TLS 1.2 not just 1.3")
  requires editing a Python file, rebuilding the bundle, signing it,
  uploading it, and upgrading the agent. A 5-minute policy change becomes
  a 30-minute release cycle.
- The agent carries policy logic — it knows what "good" looks like. This
  is unnecessary privilege: the agent only needs to report facts.
- Facts are discarded after evaluation. If a new policy is added tomorrow,
  every target must be re-scanned to evaluate against it.
- Cross-framework evaluation is impossible without re-running the entire
  bundle per framework, even though 80% of the queries are identical.

## Problem

Separate compliance checking into two distinct phases:

1. **Collection** — the agent connects to the target, runs prescribed
   queries, and streams structured facts back to the server. No
   pass/fail decisions. No policy logic.
2. **Evaluation** — the server receives facts and evaluates them against
   declarative policy rules (Rego). Produces findings (pass/fail +
   evidence + remediation per control).

## Decisions

### D1. Two-phase compliance architecture

```
Agent (customer network)              Server (DC API)
────────────────────────             ────────────────────
1. Receive directive with             
   collector manifest                 
2. Connect to target (creds           
   resolved locally or from           
   directive)                         
3. Execute collector queries          
4. Stream facts via WSS ─────────→  5. Receive + store facts
   {                                 6. Load applicable policies
     "collector_id": "mssql-config",    (Rego rules)
     "facts": {                      7. Evaluate facts against
       "tls_enabled": true,             each policy rule
       "tls_version": "1.2",        8. Produce findings
       "cipher_suites": [...],          (pass/fail + evidence)
       "sa_disabled": false,         9. Store in findings table
       "audit_login": true,         10. Emit audit events
       ...                           
     }                               
   }                                 
```

### D2. Collector definition

A collector replaces the current `check.py`. It's a script that:

- Connects to a target using provided credentials
- Runs one or more queries / commands
- Returns a structured JSON facts object
- Makes **no pass/fail decisions**

```yaml
# collectors/mssql-config/collector.yaml
id: mssql-config
name: MSSQL Server Configuration Collector
description: Collects security-relevant configuration from SQL Server
engine:
  - name: mssql
    versions: ["2019", "2022"]
framework: python
facts_schema:
  tls_enabled: boolean
  tls_version: string
  cipher_suites: string[]
  sa_account_disabled: boolean
  audit_login_enabled: boolean
  audit_login_failure: boolean
  clr_enabled: boolean
  cross_db_ownership: boolean
  xp_cmdshell_enabled: boolean
  remote_admin_connections: boolean
  default_trace_enabled: boolean
  max_error_log_files: integer
  # ... all facts this collector gathers
```

```python
# collectors/mssql-config/collect.py
import sys, json, pytds

def collect(host, port, username, password):
    conn = pytds.connect(host, port=port, user=username, password=password)
    cursor = conn.cursor()
    facts = {}

    # TLS
    cursor.execute("SELECT encrypt_option FROM sys.dm_exec_connections WHERE session_id = @@SPID")
    row = cursor.fetchone()
    facts["tls_enabled"] = row[0] == "TRUE" if row else False

    # SA account
    cursor.execute("SELECT is_disabled FROM sys.server_principals WHERE name = 'sa'")
    row = cursor.fetchone()
    facts["sa_account_disabled"] = bool(row[0]) if row else False

    # Audit
    cursor.execute("SELECT name, is_state_enabled FROM sys.server_audits")
    audits = cursor.fetchall()
    facts["audit_login_enabled"] = any(a[1] for a in audits)

    # xp_cmdshell
    cursor.execute("SELECT value_in_use FROM sys.configurations WHERE name = 'xp_cmdshell'")
    row = cursor.fetchone()
    facts["xp_cmdshell_enabled"] = bool(row[0]) if row else False

    # ... more facts ...

    conn.close()
    return facts

creds = json.loads(sys.stdin.read())
facts = collect(creds["host"], int(creds["port"]), creds["username"], creds["password"])
print(json.dumps({"collector_id": "mssql-config", "facts": facts}))
```

One collector per engine gathers ALL facts for that engine. Not one
collector per control — that would be N database connections per scan.
A single connection gathers everything.

### D3. Policy definition (Rego)

Each compliance control is a Rego rule that evaluates collected facts:

```rego
# policies/mssql-xp-cmdshell/policy.rego
package silkstrand.mssql.xp_cmdshell

import rego.v1

default result := {
    "control_id": "mssql-xp-cmdshell",
    "status": "fail",
    "severity": "high",
    "title": "Ensure xp_cmdshell is disabled",
    "remediation": "EXEC sp_configure 'xp_cmdshell', 0; RECONFIGURE;"
}

result := r if {
    not input.facts.xp_cmdshell_enabled
    r := {
        "control_id": "mssql-xp-cmdshell",
        "status": "pass",
        "severity": "high",
        "title": "Ensure xp_cmdshell is disabled",
        "evidence": {"xp_cmdshell_enabled": input.facts.xp_cmdshell_enabled}
    }
}
```

Policy metadata:

```yaml
# policies/mssql-xp-cmdshell/policy.yaml
id: mssql-xp-cmdshell
name: Ensure xp_cmdshell is disabled
severity: high
collector: mssql-config           # which collector provides the facts
fact_keys: [xp_cmdshell_enabled]  # which facts this policy reads
engine:
  - name: mssql
    versions: ["2019", "2022"]
frameworks:
  - id: cis-mssql-2022
    section: "2.7"
    title: "Ensure 'xp_cmdshell' Server Configuration Option is set to '0'"
tags: [security, configuration, command-execution]
```

### D4. Facts storage

New table for collected facts:

```sql
CREATE TABLE collected_facts (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL,
    asset_endpoint_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    collector_id TEXT NOT NULL,
    facts JSONB NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, collected_at, id)
) PARTITION BY RANGE (collected_at);

CREATE INDEX idx_facts_endpoint ON collected_facts(asset_endpoint_id, collected_at DESC);
CREATE INDEX idx_facts_scan ON collected_facts(scan_id);
```

Facts are retained for **30 days** by default (longer than findings
retention). This enables:

- **Retroactive evaluation** — add a new policy, evaluate against
  stored facts without re-scanning.
- **Drift detection** — compare facts between two collection runs.
- **Forensic analysis** — "what was the config when the finding was
  created?"

### D5. WSS message: `facts_collected`

New message type from agent → server:

```json
{
  "type": "facts_collected",
  "payload": {
    "scan_id": "<uuid>",
    "collector_id": "mssql-config",
    "facts": {
      "tls_enabled": true,
      "sa_account_disabled": false,
      "xp_cmdshell_enabled": true,
      ...
    }
  }
}
```

Server handler:
1. Store in `collected_facts`
2. Load all policies where `collector = collector_id`
3. For each policy: evaluate Rego rule with `input.facts = facts`
4. Write a finding per policy evaluation result
5. Emit audit event

### D6. Rego evaluation on the server

Embed the OPA engine as a Go library:

```go
import "github.com/open-policy-agent/opa/rego"

func EvaluatePolicy(policyRego string, facts map[string]any) (*PolicyResult, error) {
    r := rego.New(
        rego.Query("data.silkstrand.mssql.xp_cmdshell.result"),
        rego.Module("policy.rego", policyRego),
        rego.Input(map[string]any{"facts": facts}),
    )
    rs, err := r.Eval(ctx)
    // parse result → PolicyResult{ControlID, Status, Severity, ...}
}
```

OPA's Go library (`github.com/open-policy-agent/opa`) is the standard
approach — no separate OPA server needed. Evaluation is in-process,
~1ms per policy rule.

### D7. Agent runner changes

The agent's manifest runner (ADR 010 PR 3) currently iterates controls
and runs `check.py` per control. Under this ADR:

- **New bundle type**: `collector` (vs existing `compliance`)
- **Collector bundles** contain `collect.py` + `collector.yaml` —
  no policy files
- **Agent runs the collector once** per engine, gets all facts,
  streams `facts_collected` back
- **Server evaluates** N policies against the facts

The agent does NOT run Rego. It only runs the collector.

### D8. Backwards compatibility

- **Legacy bundles** (check.py with inline policy) continue to work
  via the existing manifest runner. No behavior change.
- **New collector bundles** opt in via `bundle.yaml` `type: collector`
  (vs `type: compliance`).
- **Migration path**: existing controls can be incrementally migrated.
  Extract the data-gathering portion into a collector, extract the
  pass/fail logic into Rego. Both can coexist.

### D9. Policy management in the UI

The Compliance → Controls tab evolves:

- **Controls** still exist as the user-facing concept ("Ensure
  xp_cmdshell is disabled")
- Each control now has a **policy** (Rego source) + a **collector
  reference** instead of a `check.py`
- The control detail drawer shows the Rego source (read-only in v1;
  editable for custom profiles in a follow-up)
- The Profiles tab can customize policy thresholds without touching
  collectors: override a Rego rule's default values for a specific
  tenant

### D10. Benefits of retroactive evaluation

With facts stored:

```
Day 1: Scan → collect facts → evaluate 35 CIS MSSQL policies → 35 findings
Day 2: Tenant adds a custom policy "TLS version must be 1.3"
       → re-evaluate stored facts → new finding without re-scanning
Day 3: CIS MSSQL 2022 v2.1 released (5 new controls)
       → re-evaluate stored facts → 5 new findings without re-scanning
```

The `POST /api/v1/evaluations/replay` endpoint takes a scan_id +
policy set and re-evaluates the stored facts. Zero agent involvement.

### D11. One collector per engine, many policies

A single `mssql-config` collector gathers ~50 facts. 35 CIS MSSQL
policies each read 1–3 of those facts. One database connection,
one collection run, 35 evaluations in ~35ms server-side.

This is dramatically more efficient than 35 separate check.py scripts
each opening their own database connection.

## Consequences

**Positive:**

- Agent is truly thin — runs queries, returns JSON. No policy logic.
- Policy changes deploy without agent upgrades.
- Retroactive evaluation from stored facts.
- One DB connection per scan instead of N per control.
- Rego is declarative, testable, git-diffable, auditable.
- Cross-framework evaluation from one fact set.
- Custom policy thresholds per tenant.

**Negative:**

- New dependency: OPA Go library (well-maintained, Apache 2.0,
  widely used).
- Two artifact types to manage: collectors + policies (vs one
  check.py today).
- Rego learning curve for policy authors (mitigated by examples +
  templates).
- Facts storage adds DB volume (mitigated by 30-day retention +
  partitioning).

**Scope boundary:**

- Policy editor UI (edit Rego in the browser) is a follow-on.
- Dynamic policy thresholds per tenant are a follow-on.
- Non-database collectors (OS config, file permissions) are future
  scope.
- Policy testing framework (unit tests for Rego rules) is a follow-on.

## Implementation (PR split)

1. **PR 1 — Facts schema + storage**: `collected_facts` table +
   `facts_collected` WSS handler + store methods.
2. **PR 2 — OPA integration**: embed OPA Go library, policy loader,
   evaluator, findings writer.
3. **PR 3 — Collector format**: new collector.yaml + collect.py
   schema. Convert one existing bundle (MSSQL) as proof of concept.
4. **PR 4 — Agent collector runner**: new bundle type `collector`,
   agent runs collect.py + streams facts.
5. **PR 5 — Rego policies for CIS MSSQL**: 35 Rego rules covering
   all CIS MSSQL 2022 controls.
6. **PR 6 — UI**: facts viewer in scan results, Rego source in
   control detail drawer.
7. **PR 7 — Retroactive evaluation**: `POST /evaluations/replay`
   endpoint.

## Open questions

- **OQ1.** Should collectors be engine-level (one per DB engine,
  collects everything) or domain-level (one per concern area —
  `mssql-auth`, `mssql-network`, `mssql-audit`)? Lean engine-level
  for fewer DB connections; domain-level if fact sets get too large.
- **OQ2.** Rego module namespacing: `data.silkstrand.<engine>.<control>`
  or flat `data.silkstrand.controls.<control_id>`? Lean engine-scoped
  for natural grouping.
- **OQ3.** Should the server pre-compile Rego policies at startup
  (faster eval, slower startup) or compile on demand (slower first
  eval, faster startup)? Lean pre-compile — policies change rarely,
  eval happens on every scan.
