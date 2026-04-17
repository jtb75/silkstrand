# Compliance surface — Frameworks, Controls, and Custom Profiles

Companion to ADR 010 (hybrid bundles). This plan covers the full
compliance management UX in three levels, each independently shippable.

## Motivation

Today bundles live in Settings → Bundles, a flat admin list. Operators
can't answer basic questions without digging:

- "Which controls apply to my MSSQL 2022 instances?"
- "Is TLS checking covered across all my database types?"
- "Can I run CIS PostgreSQL 16 minus the superuser checks?"

The hybrid bundle architecture (ADR 010) gives us the data model —
controls as atoms, bundles as compositions, `bundle_controls` as the
join. This plan puts a UX on top of it.

## Target state

```
Compliance (new top-level nav, Band 1 — Operator)
├─ Frameworks    (Level 1 + 2)
├─ Controls      (Level 2)
└─ Profiles      (Level 3)
```

Replaces Settings → Bundles tab (which moves back to being a raw
upload surface under Settings for admin use).

---

## Level 1 — Framework viewer (already shipped in v0.1.80)

What exists today in Settings → Bundles, promoted to the Compliance
page's Frameworks tab.

### Frameworks tab

Table columns:
- **Name** — e.g., CIS PostgreSQL 16
- **Version** — semantic version from bundle.yaml
- **Engine** — postgresql, mssql, mongodb
- **Controls** — count badge
- **Hash** — SHA256 of the uploaded tarball (truncated, full on hover)
- **Uploaded** — date
- **Actions** — View controls, Download, Delete

**Hash**: computed at upload time, stored on `bundles.tarball_hash`.
Gives operators a verification artifact — "this is the exact bundle
running on my agents."

**View controls**: expands inline (same as current Settings → Bundles
expand). Shows control table per framework.

### Schema addition for hash

```sql
ALTER TABLE bundles ADD COLUMN IF NOT EXISTS tarball_hash TEXT;
```

Populated by the upload handler: `SHA256(tarball_bytes)` before
storing.

### Nav change

Add "Compliance" to Band 1 (Operator) in the left nav, between
Findings and Scans. Remove "Bundles" from Settings tabs.

---

## Level 2 — Cross-framework control browser

The key UX unlock: controls as a searchable, filterable catalog
independent of which bundle they came from.

### Controls tab

A flat list of ALL controls across all registered bundles. Each row
shows:

- **Control ID** — globally unique, engine-prefixed (e.g., `db-tls-config`)
- **Name** — human-readable (e.g., "Ensure TLS is configured")
- **Severity** — badge (critical/high/medium/low/info)
- **Engine** — which database engines, with version ranges
- **Frameworks** — which bundles include this control (chip per
  framework, e.g., `CIS-PG-16 §3.1` `CIS-MSSQL-2022 §2.4`)
- **Tags** — filterable chips

**Filters** (top bar):
- Framework dropdown (multi-select) — "Show controls in CIS MSSQL 2022"
- Engine dropdown — "Show controls for postgresql"
- Severity dropdown — "Show high + critical only"
- Tags dropdown — "Show controls tagged 'encryption'"
- Search — free text on control ID, name, description

**Data source**: `GET /api/v1/controls` — new endpoint that reads
from `bundle_controls` with framework join. One row per unique
`(control_id, bundle_id)` pair, grouped by control_id in the UI.

### API endpoint

```
GET /api/v1/controls?framework=cis-mssql-2022&engine=mssql&severity=high&tag=encryption&q=tls
```

Response:
```json
{
  "items": [
    {
      "control_id": "db-tls-config",
      "name": "Ensure TLS is configured",
      "severity": "high",
      "engine": "mssql",
      "engine_versions": ["2019", "2022"],
      "tags": ["encryption", "network", "tls"],
      "frameworks": [
        { "bundle_id": "...", "bundle_name": "CIS MSSQL 2022", "section": "2.4" },
        { "bundle_id": "...", "bundle_name": "CIS PostgreSQL 16", "section": "3.1" }
      ]
    }
  ],
  "total": 79
}
```

### Store method

```sql
SELECT bc.control_id, bc.name, bc.severity, bc.engine, bc.engine_versions, bc.tags,
       b.id as bundle_id, b.name as bundle_name, bc.section
  FROM bundle_controls bc
  JOIN bundles b ON b.id = bc.bundle_id
 WHERE ($1 = '' OR b.framework = $1)
   AND ($2 = '' OR bc.engine = $2)
   AND ($3 = '' OR bc.severity = $3)
 ORDER BY bc.control_id, b.name
```

The handler groups by `control_id` and nests the framework mappings.

### Control detail drawer

Click a control row → drawer showing:
- Full metadata from control.yaml
- Which frameworks include it (with section references)
- Which engine + version ranges
- Description + tags
- **Findings for this control** — query `findings WHERE source_id = <control_id>` to show which assets are failing this specific control, across all frameworks

This is the cross-framework view: "which assets fail TLS config,
regardless of whether we discovered them via the PostgreSQL or MSSQL
benchmark."

---

## Level 3 — Custom compliance profiles

Operators select controls from the catalog to build tenant-specific
bundles. Replaces the "upload a tarball you built locally" workflow
with a point-and-click experience.

### Profiles tab

Table of custom profiles (tenant-scoped bundles):
- **Name** — e.g., "ACME Database Hardening"
- **Based on** — which framework(s) the controls came from
- **Controls** — count
- **Version** — auto-incremented on each edit
- **Status** — Draft / Published / Active
- **Actions** — Edit, Publish, Delete

### Create profile flow

1. Click `[ + New Profile ]`
2. **Name + description** form
3. **Control picker**: same filter UI as the Controls tab, but with
   checkboxes. Pre-select all controls from a framework as a starting
   point, then deselect what you don't want:
   ```
   Start from: [ CIS PostgreSQL 16 ▼ ]  → pre-checks all 33 controls
   
   ☑ db-tls-config          high    §3.1
   ☑ pg-hba-config           high    §4.1
   ☐ pg-default-roles        low     §7.2  ← unchecked by operator
   ☑ pg-log-connections      medium  §5.3
   ...
   
   Selected: 32 / 33 controls
   ```
4. Click `[ Save as Draft ]`

### Publish flow

A draft profile exists in the DB but isn't usable for scans. Publishing:

1. Server assembles the tarball from the selected controls (same logic
   as `scripts/build-bundle.sh` but server-side).
2. Signs the tarball with the configured bundle signing key (requires
   `BUNDLE_SIGNING_KEY` secret).
3. Stores in GCS / local storage.
4. Registers as a `bundles` row with `source = 'custom'`.
5. Profile status → Published.

Now it appears in the scan definition bundle dropdown like any other
bundle.

### Edit + versioning

Editing a published profile creates a new draft version. Publishing
bumps the version and rebuilds the tarball. Agents on the old version
re-download on next scan (version mismatch triggers cache invalidation).

### Schema additions

```sql
CREATE TABLE compliance_profiles (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT,
  base_framework TEXT,               -- optional: which framework seeded it
  status TEXT NOT NULL DEFAULT 'draft',  -- draft | published
  version INT NOT NULL DEFAULT 1,
  bundle_id UUID REFERENCES bundles(id) ON DELETE SET NULL,  -- set when published
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by UUID,
  UNIQUE (tenant_id, name)
);

CREATE TABLE profile_controls (
  profile_id UUID NOT NULL REFERENCES compliance_profiles(id) ON DELETE CASCADE,
  control_id TEXT NOT NULL,
  PRIMARY KEY (profile_id, control_id)
);
```

### Server-side bundle builder

New package `api/internal/bundler/` that:
1. Reads `profile_controls` for the profile
2. For each control_id, reads the control files from the master
   control directory (GCS or local — wherever controls are stored)
3. Assembles the tarball in-memory
4. Signs with `BUNDLE_SIGNING_KEY`
5. Stores to GCS / local
6. Upserts the `bundles` row

This is the server-side equivalent of `scripts/build-bundle.sh`.

### API endpoints

```
GET    /api/v1/compliance-profiles
POST   /api/v1/compliance-profiles
GET    /api/v1/compliance-profiles/{id}
PUT    /api/v1/compliance-profiles/{id}
DELETE /api/v1/compliance-profiles/{id}
POST   /api/v1/compliance-profiles/{id}/publish
POST   /api/v1/compliance-profiles/{id}/controls    — batch set control list
GET    /api/v1/compliance-profiles/{id}/controls    — list selected controls
```

---

## Implementation plan

### Level 1 enhancements (1 PR)
- Add `tarball_hash` to bundles table + compute at upload
- Promote Frameworks tab to Compliance page
- Add hash column to the table
- Nav: add "Compliance" between Findings and Scans

### Level 2 (2 PRs)
- PR A: `GET /api/v1/controls` endpoint with filters + grouping
- PR B: Controls tab UI with filters, search, control detail drawer
  with cross-framework findings

### Level 3 (3 PRs)
- PR A: `compliance_profiles` + `profile_controls` schema + CRUD API
- PR B: Server-side bundle builder (`api/internal/bundler/`)
- PR C: Profiles tab UI with control picker, draft/publish flow

### Sequencing

```
Level 1 ──> Level 2A ──> Level 2B ──> Level 3A ──> Level 3B ──> Level 3C
  (1 PR)     (1 PR)       (1 PR)      (1 PR)       (1 PR)       (1 PR)
```

Each level is independently shippable. Level 2 delivers the most
value per effort (cross-framework control visibility). Level 3 is the
power-user unlock.

---

## Design principles

- **Controls are the lingua franca.** Every UX surface — frameworks,
  profiles, findings, scan results — references controls by their
  globally unique ID. An operator can trace from a finding → control
  → which frameworks require it → which assets are failing.

- **Frameworks are immutable compositions.** CIS PostgreSQL 16 is what
  it is. You can't edit its control list (that comes from the CIS
  benchmark). Custom profiles are the editable layer.

- **Profiles are tenant-scoped.** Each tenant builds their own
  compliance posture from the available control catalog. Two tenants
  can have different profiles even though they share the same
  underlying controls.

- **Signing is non-negotiable.** Every bundle that runs on an agent —
  whether from a standard framework or a custom profile — is signed.
  The publish flow enforces this.

---

## Out of scope

- Automatic CIS benchmark import (controls are hand-authored)
- Control marketplace / sharing between tenants
- Remediation automation (auto-fix a failing control)
- Continuous compliance scoring / dashboards (follow-on to Level 2)
- Control dependencies / prerequisite ordering beyond manifest order
