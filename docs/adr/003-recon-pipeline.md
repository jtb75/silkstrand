# ADR 003: Recon + compliance pipeline (active design)

**Status:** Proposed (active design)
**Date:** 2026-04-13
**Supersedes:** [ADR 002](./002-recon-pipeline.md)
**Related:** [ADR 004](./004-credential-resolver.md) (credential resolver)

---

## Context

ADR 002 framed the evolution from compliance scanner to recon + compliance
platform and parked the implementation. This ADR captures the concrete
design decisions made during the April 2026 design review and puts the
work on an active track.

The motivating customer question has not changed: "what do I have?"
precedes "is it compliant?" in almost every onboarding. The path from
zero to compliance scans is substantially shorter if the agent can
discover assets inventory-style before the admin hand-builds targets.

## Decisions

### D1. Scan composition — hybrid

One new scan type, **Discovery scan**, couples Stages 1 and 2 (network
reachability + service/fingerprint/CVE enrichment). They share infra
(PD toolchain), share data (one asset row enriched in place), and
almost always run together.

**Compliance scan** (Stage 3) stays as today — separate runs, per-target,
credentialed.

Two top-level scan types, not three. Clean separation at the seam that
matches workflow cadence: discovery on a fast loop (daily/weekly),
compliance on its own cadence (per-target, audit-driven).

### D2. Promote to compliance — rule-based, suggest by default

Every promote-to-compliance path goes through a **correlation rule**.
Rules declaratively match discovered-asset attributes and map them to
a bundle:

```yaml
rules:
  - match: { service: postgresql, version: "16.*" }
    bundle: cis-postgresql-16
    mode: suggest           # suggest | auto
```

- `suggest` (default): a candidate target is surfaced in the UI for
  admin approval.
- `auto`: target created without confirmation, contingent on credentials
  resolving (see ADR 004).

Rules are the metadata that connects "discovered tech X" to "available
bundle Y." Without them the Assets page cannot distinguish compliance
candidates from general inventory.

### D3. Asset visibility — single Assets page

A single **Assets** page is the centerpiece of the recon product.
Every discovered asset appears with columns for service, version,
CVE count, and compliance status. Filter chips drive the common views
(`With CVEs`, `Compliance candidates`, `Failing compliance`,
`Recently changed`, `New this week`).

Non-compliance assets (e.g. Confluence with a known CVE, Jenkins with
a medium-severity finding) are first-class inventory, not hidden. The
product is honest about the value of discovery beyond compliance
mapping.

The existing **Targets** page remains initially. Long-term it likely
becomes a saved filter on Assets ("assets with active compliance
targets"), but that UX migration is out of scope for v1.

### D4. Change tracking — current-state + event log

```sql
discovered_assets (
  id UUID PK,
  tenant_id UUID NOT NULL,
  ip INET NOT NULL,
  port INT NOT NULL,
  hostname TEXT,
  service TEXT,
  version TEXT,
  technologies JSONB,
  cves JSONB,
  compliance_status TEXT,
  source TEXT NOT NULL,           -- 'manual' | 'discovered'
  first_seen TIMESTAMPTZ NOT NULL,
  last_seen  TIMESTAMPTZ NOT NULL,
  last_scan_id UUID,
  UNIQUE(tenant_id, ip, port)
);

asset_events (
  id UUID PK,
  tenant_id UUID NOT NULL,
  asset_id UUID NOT NULL REFERENCES discovered_assets(id),
  scan_id UUID,
  event_type TEXT NOT NULL,
    -- new_asset | asset_gone | new_cve | cve_resolved
    -- | version_changed | port_opened | port_closed
    -- | compliance_pass | compliance_fail
  payload JSONB,
  occurred_at TIMESTAMPTZ NOT NULL
);
```

Current-state table reads are cheap (one row per asset). `first_seen` /
`last_seen` make "new this week" and "recently stale" queries trivial.
Event log captures the deltas that matter for security — drift reports,
audit trails, notifications — without bloating the asset table with
unchanged snapshots.

Trim `asset_events` at ~3 years by default, configurable per tenant.
`discovered_assets` rows persist for the tenant's lifetime.

### D5. Target types (v1)

All under `target_type: network_range` with shape variants:

| Shape | Example |
|---|---|
| CIDR | `10.0.0.0/24` |
| IP range | `10.0.0.5-10.0.0.50` |
| Single IP | `10.0.0.5` |
| Hostname | `internal-app.corp.local` (resolved at scan time) |

**Deferred to v1.1 (fast-follow):** CSV / paste-list host import.

**Deferred to v2:** DNS zone enumeration (`target_type: dns_zone`).
Separate engineering effort — multiple discovery techniques (AXFR,
brute-force, AD LDAP), asset model handling for name-without-IP, rate
limiting responsibility. Demand-driven.

### D6. Manual + discovered unification

`discovered_assets` is the single source of truth. **Manual target
creation writes an asset row first** (`source: manual`), then a target
row referencing it. Discovery upserts on `(tenant_id, ip, port)` —
whichever path got there first owns the `source` value; subsequent
passes enrich in place (fingerprint, CVEs, versions, `last_seen`).

```sql
targets (
  id UUID PK,
  asset_id UUID NOT NULL REFERENCES discovered_assets(id),
  credential_source_id UUID REFERENCES credential_sources(id),  -- ADR 004
  bundle_id UUID NOT NULL,
  ...
);
```

One-time migration backfills an asset row for every existing target
and populates `targets.asset_id`.

A manually-added target is no longer a data island: discovery enriches
it automatically on the next run. A `locked` flag on assets (v2) can
opt out for customers who want pristine manual inventory — defer until
requested.

### D7. Recon runtime — separate framework

Bundles declare `framework`. Today's compliance bundles are
`framework: python`. Recon introduces `framework: recon-pipeline`
with a separate Go runner in the agent.

Result schemas diverge:

- `silkstrand-v1` (compliance, exists): controls with PASS/FAIL/ERROR
  semantics.
- `silkstrand-discovery-v1` (recon, new): assets with service/version/
  CVE data, plus an `events[]` array that flows directly into
  `asset_events`.

Shared bundle mechanics — tarball layout, manifest.yaml, `vendor_dir`,
GCS distribution, SHA-256 verification, Ed25519 signing, WSS transport,
cache invalidation — reused without change. Only the runtime interior
and the result schema differ.

Recon runner lives at `agent/internal/runner/recon.go`. Invokes PD
tools (naabu, httpx, nuclei) either as subprocesses or as embedded
libraries (TBD at implementation).

**Authoring docs split:**
- `docs/bundle-authoring.md` (existing) — compliance YAML controls.
- `docs/recon-pipeline-authoring.md` (new, written alongside R1) —
  stage composition, rate limits, tool configuration.

### D8. Stage 1 scope — L3 in v1, cloud APIs in v1.1

**v1** — `target_type: network_range` only. Network reachability via
`naabu` + HTTP/TLS fingerprinting via `httpx` + CVE matching via
`nuclei` against discovered services.

**v1.1** — Add `target_type: aws_account` alongside ADR 004's
Phase C1 (AWS Secrets Manager integration). Walks AWS APIs
(`DescribeDBInstances`, `ListBuckets`, `ListFunctions`, etc.) and
writes the results into the same `discovered_assets` table. The
credential Phase 2 work already walks most of these APIs for
`MasterUserSecret.SecretArn` auto-binding; extending it to populate
the inventory is ~30% additional work.

AWS API calls default to **agent-side** (agent has IAM role, makes
calls from customer network egress) rather than API-server-side
(cross-account trust to SilkStrand's cloud account). Keeps the
"agent inside customer network" architecture consistent.

**v2+** — Azure, GCP, Kubernetes, SaaS targets. Same pattern
(new `target_type` per provider). Demand-driven.

### D9. Ingestion path — streaming WSS

Recon results stream back over the existing agent WSS channel with
new message types:

```
asset_discovered      (new)  — one or more asset dicts
discovery_completed   (new)  — scan finished, no more assets
scan_error            (existing) — scan failed; partial results retained
```

API processes each `asset_discovered` message immediately (upsert +
append events). Discovery scan status transitions: `pending → running`
on first message, `running → completed` on `discovery_completed`,
`running → failed` on `scan_error`.

Long-running recon scans (a /16 can take hours) give live progress
in the UI. Partial failure preserves partial findings. Same
transport, auth, and reconnect logic as compliance scans.

### D10. Data store — stay relational

All asset / finding / event data lives in Postgres alongside the
existing compliance schema. JSONB for variable content (CVE lists,
fingerprint dictionaries, nuclei raw output). Recursive CTEs for the
rare graph-shaped queries.

Graph database (Neo4j, Memgraph, etc.) is deferred. The queries we
actually need for the first 12+ months are tabular (inventory,
filtering, time-range diffs). Graph-shaped queries (blast radius,
lateral movement paths, transitive dependency analysis) are tied to
product features that are out of scope for this ADR. If and when
such a feature lands on the roadmap, first evaluate `pg_age`
(openCypher on Postgres) before introducing a second database.

## Schema summary

```
discovered_assets       — current-state inventory, one row per (tenant, ip, port)
asset_events            — append-only security-relevant changes
targets                 — existing, refactored to reference assets
correlation_rules       — new, drives promote-to-compliance
credential_sources      — see ADR 004
```

## Implementation phases

| Phase | Scope | Prereq |
|---|---|---|
| **R0** | Asset/target schema migration; `credential_source_id` on targets | ADR 004 Phase C0 |
| **R1** | L3 discovery (`target_type: network_range`), correlation rules, Assets page, manual/discovered unification | R0 |
| **R1.1** | Host list import target source | R1 |
| **R2** | AWS cloud discovery (`target_type: aws_account`); cloud-native credential binding | R1, ADR 004 Phase C1 |
| **R3+** | Vault credential resolution (ADR 004 Phase C3); DNS zone enumeration; Azure/GCP cloud discovery | Demand-driven |

## What we are not building

Unchanged from ADR 002, reiterated for clarity:

- Generic external attack surface management (subfinder, internet-wide
  scans). We stay inside customer networks.
- EDR / endpoint telemetry. Different product.
- A from-scratch vulnerability database. Lean on Nuclei templates,
  NVD, KEV.
- Graph-native attack path analysis. Possible future, not this ADR.

## Open items deferred to implementation

- Exact batch size per `asset_discovered` message (start with 1–10,
  batch up based on throughput observation).
- Scheduler UX for recurring discovery scans (per-target cadence,
  maintenance windows, rate-limit profiles).
- Correlation-rule authoring UI (start with config file /
  admin-only page; promote to rich editor later).
- Cross-account AWS target topology (do we support one-target-per-
  account, or tenant-level fanout across OUs?). Punt until we have a
  multi-account customer.
- Whether agent embeds PD tools as libraries or invokes as subprocesses.
- Runtime install strategy for PD binaries — track in a separate note;
  leaning toward agent-managed install into `/var/lib/silkstrand/runtimes/`
  rather than per-bundle vendoring.

## Consequences

- **Positive**: full-picture product on day one of customer onboarding
  (inventory + findings), clean abstraction that keeps compliance from
  being polluted by recon concerns, reuses existing bundle
  infrastructure, grows naturally with more cloud providers without
  architecture changes.
- **Negative**: additional data model complexity (assets + events +
  rules), second bundle framework to maintain, new tool chain to
  ship (PD binaries), broader attack surface from port scanning
  capability that must be controlled (allowlist CIDRs, rate-limit
  profiles, customer consent gates).
- **Neutral**: product name / site copy likely needs revisions once
  R2 ships. Compliance-only customers never see recon surfaces.
