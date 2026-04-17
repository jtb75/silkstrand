# ADR 010: Hybrid bundle architecture — controls + computed compositions

**Status:** Proposed
**Date:** 2026-04-17
**Related:** [ADR 003](./003-recon-pipeline.md) (recon pipeline — bundle execution),
[ADR 007](./007-findings-scheduler.md) (findings table — `source` + `source_id`
per control), [ADR 009](./009-service-detection.md) (service detection — bundles
target detected services).

---

## Context

SilkStrand ships three CIS compliance bundles today: `cis-postgresql-16`,
`cis-mssql-2022`, `cis-mongodb-8`. Each is a monolithic Python script
(`content/checks.py`) containing every control for that benchmark. This
creates three problems:

1. **Duplication.** Controls like "check TLS configuration," "check
   password policy," and "check audit logging" appear in every bundle
   with minor variations. A fix to the TLS check must be applied N
   times.

2. **Version forking.** When CIS publishes PostgreSQL 17, we fork the
   entire postgresql-16 bundle even though ~80% of controls are
   identical. Two bundles, double the maintenance.

3. **No cross-framework reporting.** "Show me every asset failing the
   TLS control" requires querying N bundles and correlating results.
   The findings table already has `source_id` (control_id), but the
   control IDs are bundle-internal strings with no shared namespace.

## Problem

Design a bundle architecture that:

- Treats individual **controls** as the atomic authoring unit.
- **Composes** controls into framework-specific bundles at build time.
- Preserves the existing **signed-artifact trust model** (one signature
  per bundle tarball).
- Requires **no agent-side changes** to the download/cache/verify flow
  (agents still fetch tarballs from GCS).
- Changes the agent's **execution model** from "run one entrypoint" to
  "iterate controls from a manifest."
- Produces **per-control findings** so cross-framework queries work.
- Supports **custom compliance profiles** (tenant selects a subset of
  controls).

## Decisions

### D1. Control as the atomic unit

A control is a single check with metadata:

```yaml
# controls/tls-config/control.yaml
id: db-tls-config
name: Ensure TLS is configured
description: Verify that TLS is enabled and uses strong ciphers.
severity: high
engine:
  - name: postgresql
    versions: ["14", "15", "16"]
  - name: mssql
    versions: ["2019", "2022"]
  - name: mongodb
    versions: ["6", "7", "8"]
frameworks:
  - id: cis-postgresql-16
    section: "3.1"
    title: "Ensure SSL is enabled"
  - id: cis-mssql-2022
    section: "2.4"
    title: "Ensure TLS is configured"
  - id: cis-mongodb-8
    section: "3.2"
    title: "Ensure TLS/SSL is enabled"
tags: [encryption, network, tls]
requires_auth: true
```

Each control has:
- A **globally unique ID** (`tls-config`). Immutable once published.
- One or more **framework mappings** — which benchmarks this control
  satisfies and which section/requirement it covers.
- An **engine list** — which database engines this control applies to.
- An **entrypoint** — a Python script (or future: any executable) that
  receives connection info + returns a structured result.
- **Tags** for grouping and filtering.

### D2. Control entrypoint contract

Each control is a directory:

```
controls/tls-config/
  control.yaml         # metadata per D1
  check.py             # entrypoint
  vendor/              # optional deps (same pattern as today)
```

`check.py` receives credentials + connection info via the same FD-pipe
mechanism the current runner uses. It returns a JSON result:

```json
{
  "control_id": "tls-config",
  "status": "fail",
  "severity": "high",
  "title": "Ensure TLS is configured",
  "evidence": {"tls_enabled": false, "cipher_suites": []},
  "remediation": "Enable TLS in postgresql.conf: ssl = on"
}
```

This is the same shape as today's per-check result inside `checks.py`,
just one result per control instead of an array from one monolithic
script.

### D3. Bundle = computed composition

A bundle is a **manifest + the controls it includes**, assembled at
build time:

```yaml
# bundles/cis-postgresql-16/bundle.yaml
id: cis-postgresql-16
name: CIS PostgreSQL 16 Benchmark
version: 2.0.4
framework: cis-postgresql-16
engine: postgresql
controls:
  - tls-config
  - password-policy
  - audit-logging
  - pg-hba-config
  - pg-auth-method
  - pg-log-connections
  - pg-superuser-check
  - pg-default-roles
```

The `controls:` list is computed by scanning `controls/*/control.yaml`
for entries where `frameworks[].id == "cis-postgresql-16"`. A build
script assembles the tarball:

```
bundles/cis-postgresql-16/
  bundle.yaml           # manifest
  controls/
    tls-config/         # copied from controls/tls-config/
      control.yaml
      check.py
    password-policy/
      control.yaml
      check.py
    ...
```

The tarball is signed and uploaded to GCS. The agent downloads it like
any other bundle. The bundle.yaml tells the runner which controls to
execute and in what order.

### D4. Shared controls across bundles

A control like `tls-config` exists once in the source tree at
`controls/tls-config/`. When bundles are assembled, the control is
**copied** into each bundle that includes it. No symlinks, no runtime
dependency resolution. Each bundle is fully self-contained after
assembly.

This preserves the signing model: each tarball is a complete artifact
that can be verified independently.

### D5. Agent runner changes

The agent's `runner.PythonRunner` currently does:

```go
// Today: run the bundle's single entrypoint
result, err := runner.Execute(bundlePath, "content/checks.py", creds)
```

After this ADR:

```go
// Read the manifest
manifest := readBundleManifest(bundlePath)

// Execute each control
var results []ControlResult
for _, controlID := range manifest.Controls {
    controlPath := filepath.Join(bundlePath, "controls", controlID, "check.py")
    result, err := runner.Execute(bundlePath, controlPath, creds)
    results = append(results, result)
    // Stream per-control result back to server as it completes
}
```

**Per-control streaming**: instead of waiting for all controls to
finish, the agent streams each control's result as it completes via
the existing `scan_results` WSS message. The server writes a finding
per control. This gives the operator real-time progress in the UI.

**Backwards compatibility**: if `bundle.yaml` doesn't exist (legacy
bundle), fall back to the old `content/checks.py` entrypoint. This
lets the transition happen incrementally.

### D6. Bundle builder CLI

A `make bundle` target (or `scripts/build-bundle.sh`) that:

1. Reads `bundles/<name>/bundle.yaml` for the control list.
2. For each control ID, copies `controls/<id>/` into the output.
3. Signs the tarball with the bundle signing key.
4. Outputs `<name>-<version>.tar.gz` + `<name>-<version>.tar.gz.sig`.

```bash
make bundle BUNDLE=cis-postgresql-16
# → dist/cis-postgresql-16-2.0.4.tar.gz
# → dist/cis-postgresql-16-2.0.4.tar.gz.sig
```

### D7. Bundle upload API

New endpoint: `POST /api/v1/bundles/upload`

- Accepts multipart form: `tarball` (the signed .tar.gz) + `signature`
  (the .sig file).
- Server verifies the signature against a configured public key.
- Extracts `bundle.yaml` from the tarball to read metadata.
- Upserts the `bundles` DB row (id, name, version, framework,
  engine, control_count).
- Stores the tarball in GCS at the path agents download from.
- Returns the created/updated bundle record.

This replaces the current manual "upload to GCS + insert DB row"
workflow.

### D8. Findings per control

The findings table (ADR 007 D1) already has:

```
source       TEXT  -- bundle id (e.g., 'cis-postgresql-16')
source_id    TEXT  -- control id (e.g., 'tls-config')
source_kind  TEXT  -- 'bundle_compliance'
```

No schema change needed. Each control produces one finding. The
`source_id` is the control's globally unique ID from `control.yaml`,
not a bundle-internal string. This enables cross-framework queries:

```sql
SELECT * FROM findings
 WHERE source_id = 'tls-config'
   AND status = 'open';
-- Returns all assets failing the TLS check, regardless of which
-- bundle (postgresql, mssql, mongodb) produced the finding.
```

### D9. Custom compliance profiles (future)

With controls as the atomic unit, a tenant can create a custom
bundle by selecting specific controls:

```yaml
# Custom profile
id: acme-database-hardening
controls:
  - tls-config
  - password-policy
  - audit-logging
  # Skip: pg-default-roles (not relevant to us)
```

The build system assembles this like any other bundle. This is out
of scope for the initial implementation but the architecture supports
it natively.

### D10. Bundle versioning

Bundles carry a semantic version in `bundle.yaml`. The `bundles` DB
table already has a `version` column. When a new version is uploaded,
the existing row is updated (not a new row). Agents cache by
`name + version`; a version bump triggers a re-download.

Control-level versioning is implicit: the control's content is what's
in the bundle at that version. No separate version per control.

### D11. DB schema changes

Extend the `bundles` table:

```sql
ALTER TABLE bundles
  ADD COLUMN IF NOT EXISTS engine TEXT,
  ADD COLUMN IF NOT EXISTS control_count INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS gcs_path TEXT;
```

New table for control metadata (so the API can list available controls
without downloading the tarball):

```sql
CREATE TABLE bundle_controls (
  bundle_id UUID NOT NULL REFERENCES bundles(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,           -- globally unique, engine-prefixed (e.g., db-tls-config, pg-hba-config)
  name TEXT NOT NULL,
  severity TEXT,
  section TEXT,                       -- framework section reference (e.g., "3.1")
  engine TEXT NOT NULL,               -- postgresql, mssql, mongodb, mysql, ...
  engine_versions JSONB NOT NULL DEFAULT '[]'::jsonb,  -- ["14","15","16"] or ["*"] for all
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  PRIMARY KEY (bundle_id, control_id)
);

CREATE INDEX idx_bundle_controls_engine ON bundle_controls(engine);
CREATE INDEX idx_bundle_controls_control ON bundle_controls(control_id);
```

Populated at upload time from the control.yaml files inside the
tarball.

## Consequences

**Positive:**

- Write each control once, compose into N bundles. TLS check fixed
  once → all frameworks benefit.
- Cross-framework queries on `findings.source_id` work natively.
- Version bumps (CIS PG 14 → 16) only touch changed controls.
- Custom compliance profiles become possible.
- Signed-artifact trust model preserved (one sig per bundle tarball).
- No agent download/cache/verify changes — still fetches tarballs.

**Negative:**

- Agent runner needs manifest-aware execution loop (D5). Small change
  but touches the hot path.
- Build step added (D6). Today bundles are hand-authored directories;
  now they need assembly.
- Existing three bundles need to be decomposed into individual
  controls. One-time migration cost.

**Scope boundary:**

- No custom profile UI (D9) in this ADR — architecture supports it
  but the implementation is a follow-on.
- No bundle marketplace / sharing between tenants.
- No versioning UI beyond listing the current version.
- No automatic CIS benchmark import — controls are hand-authored.

## Implementation (PR split)

1. **PR 1 — Control + manifest schema**: Define `control.yaml` and
   `bundle.yaml` schemas. Decompose the three existing CIS bundles
   into individual controls. Write `bundle.yaml` manifests.
2. **PR 2 — Bundle builder**: `make bundle` script that assembles
   + signs tarballs from controls + manifests.
3. **PR 3 — Agent runner**: Manifest-aware execution loop with
   per-control result streaming. Backwards-compatible fallback.
4. **PR 4 — Bundle upload API + DB changes**: `POST /bundles/upload`
   with signature verification, GCS storage, `bundle_controls` table.
5. **PR 5 — UI**: Bundle management page (list bundles, upload,
   view controls per bundle).

## Resolved questions

- **Q1. Control ID namespacing** — globally unique with engine prefix
  convention (`db-tls-config`, `pg-hba-config`, `mssql-audit-login`).
  Engine + version applicability tracked in `control.yaml` and
  persisted to `bundle_controls.engine` + `engine_versions` so the
  API can answer "which controls apply to MSSQL 2022?" without
  downloading tarballs.
- **Q2. Execution order** — manifest order = execution order. No
  formal dependency graph. If a future control needs ordering
  guarantees, the manifest author places it after its prerequisite.
- **Q3. Builder location** — both `make bundle` (local dev) and a
  GitHub Actions step (CI releases). Same shell script, different
  triggers.
