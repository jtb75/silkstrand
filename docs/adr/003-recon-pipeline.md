# ADR 003: Recon + compliance pipeline (active design)

**Status:** Accepted (R0–R1c shipped v0.1.44, asset-first refactor v0.1.49)
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

### D2. Rule engine — generalized `match → action`

A single declarative rule engine drives every reactive behavior in the
recon pipeline. Rules match against asset attributes (and, for event-
triggered rules, the event payload) and emit actions. Promote-to-
compliance is one action; notification dispatch (D12) and one-shot
scans (D13) are others. One grammar, one authoring surface, one
evaluation pass per `asset_discovered` / `asset_event`.

```yaml
rules:
  # Promote a discovered Postgres 16 to a compliance target.
  - name: cis-postgres-16-suggest
    match: { service: postgresql, version: "16.*" }
    on: asset_discovered
    actions:
      - type: suggest_target
        bundle: cis-postgresql-16

  # Auto-create a compliance target for any RDS instance with a
  # bound MasterUserSecret.
  - name: rds-aurora-auto
    match: { source: aws_rds, has_credential_binding: true }
    on: asset_discovered
    actions:
      - type: auto_create_target
        bundle: cis-postgresql-16

  # Shadow IT: alert when RDP shows up on a segment it shouldn't.
  - name: rdp-out-of-policy
    match: { service: rdp, ip: "10.50.0.0/16", not_in: "10.50.10.0/24" }
    on: asset_discovered
    actions:
      - type: notify
        channel: slack-secops
        severity: high

  # Drift: alert when a host disappears from a known-critical segment.
  - name: critical-host-gone
    match: { ip: "10.0.0.0/24" }
    on: asset_event
    when: { event_type: asset_gone }
    actions:
      - type: notify
        channel: pagerduty-oncall
        severity: critical
```

Action types (initial set):

| `type`              | Effect                                                                                 |
|---------------------|----------------------------------------------------------------------------------------|
| `suggest_target`    | Candidate compliance target surfaced in UI for admin approval.                         |
| `auto_create_target`| Target created without confirmation; depends on credentials resolving (ADR 004).       |
| `notify`            | Dispatch via the channel abstraction (D12).                                            |
| `run_one_shot_scan` | Schedule an immediate scan of a bundle against the matching asset set (D13).           |

The `on` field selects the trigger. `asset_discovered` fires on every
upsert into `discovered_assets`; `asset_event` fires on every row
appended to `asset_events` (with an optional `when` filter on the
event type).

Rule evaluation is per-tenant. Rules are stored in `correlation_rules`
(versioned, with audit trail) and authored via the admin UI or
config-file import (early phases: file import only).

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

### D11. Customer-controlled scan allowlist (added 2026-04-13, audit 3.2)

The agent will only initiate discovery against CIDRs / hostnames the
customer has explicitly enrolled in a **local allowlist file** on the
agent host (`/etc/silkstrand/scan-allowlist.yaml` by default,
configurable via `SILKSTRAND_SCAN_ALLOWLIST_PATH`). The SaaS cannot
override or extend this allowlist — directives that target
out-of-allowlist ranges are rejected by the agent before any packet
goes out.

```yaml
# /etc/silkstrand/scan-allowlist.yaml
allow:
  - 10.0.0.0/16
  - 192.168.50.0/24
  - corp-internal.example.com
deny: []          # optional, evaluated after allow
rate_limit_pps: 1000   # global packets-per-second ceiling
```

Two-key principle: a compromised SaaS account cannot weaponize
customer agents into a lateral-movement / DoS tool against the
customer's own network. The customer's local file is the ultimate
authority.

A global hard-coded packets-per-second cap also lives in the recon
runner code as defense-in-depth against allowlist misconfiguration.

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

### D12. Notifications — pluggable channels

`asset_events` without an outbound channel is a query-only feature.
For shadow-IT detection, drift alerting, and zero-day response signals
to be useful, push notifications are required.

A single `notification_channels` table per tenant holds channel
configurations. Each channel has a `type` and a JSONB `config` shaped
per type:

| `type`        | `config` keys                                  | Phase |
|---------------|------------------------------------------------|-------|
| `webhook`     | `url`, `secret` (HMAC sig), optional headers   | R1c   |
| `slack`       | `webhook_url` (Slack incoming webhook)         | R1c   |
| `email`       | `to[]`, `from` (uses Resend, ADR-existing)     | R1c   |
| `pagerduty`   | `routing_key`                                  | R1.1  |
| `opsgenie`    | `api_key`, `team`                              | R2+   |

The D2 rule engine's `notify` action references a channel by name and
includes a severity (`info`/`low`/`medium`/`high`/`critical`) plus a
templated message body.

Delivery is best-effort with retry: 3 attempts with exponential
backoff, then logged to a `notification_deliveries` table for audit
and UI surfacing ("last 100 alerts" view). No on-call paging escalation
inside SilkStrand — that's PagerDuty's job; we just hand off.

Webhook signing: every webhook payload is HMAC-SHA256 signed with the
channel's secret, header `X-SilkStrand-Signature`. Customers verify on
their end. Same pattern as Stripe / GitHub webhooks.

### D13. Asset sets and one-shot scans

Today every scan addresses a single `target_id`. That doesn't fit
either Shadow IT remediation ("scan all out-of-policy RDP hosts") or
zero-day response ("run Log4Shell template against everything HTTP-
ish"). We add two complementary primitives:

**Asset sets** — saved queries over `discovered_assets`:

```sql
asset_sets (
  id UUID PK,
  tenant_id UUID NOT NULL,
  name TEXT NOT NULL,
  predicate JSONB NOT NULL,    -- structured filter, same grammar as
                               -- D2 rule `match` clauses
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);
```

A predicate evaluates against the asset table. Sets can be referenced
by name in rules, in scheduled scans, and from the UI ("scan this
set"). Sets are dynamic — membership is computed at scan dispatch
time, not snapshotted at creation.

**One-shot scans** — a single bundle dispatched against an asset set
in one operation, distinct from per-target compliance scans:

```
POST /api/v1/scans/one-shot
{
  "bundle_id": "log4shell-response",
  "asset_set_id": "set-uuid",            // OR
  "asset_set_predicate": { ... },         // ad-hoc
  "max_concurrency": 50,
  "rate_limit_pps": 500
}
```

Returns a parent `one_shot_scan_id`; the API fans out to per-asset
scan rows so existing scan-result schemas, audit trails, and the
agent WSS dispatch all reuse current code paths. A new
`one_shot_scans` table holds the parent metadata (kicked off by /
dispatched at / completion summary).

The D2 `run_one_shot_scan` action is how a rule (e.g., "new CVE
template uploaded for tag X") triggers this automatically.

### D14. Template catalog — SilkStrand-curated, tenant-selectable (added 2026-04-13)

Two-layer model for nuclei template management, mirroring the
customer-control principle from D11:

**Layer 1 — SilkStrand catalog (operational hygiene).** SilkStrand
maintains the set of templates available to all tenants. Curation
defaults: drop `fuzzing/`, `headless/`, `dns/`, `ssl/`; drop tags
`intrusive`, `dast`, and any template requiring third-party API keys
we don't ship (`shodan`, `censys`, `virustotal`, `chaos`, `github`).
Categories kept: `cves/`, `vulnerabilities/`, `exposures/`,
`misconfiguration/`, `network/`, `default-logins/`, `exposed-panels/`,
`technologies/`. The signed bundle distribution from D7 ships the
curated subset; `info`-severity templates are kept for fingerprint
enrichment (the run-time `-severity` flag, not bundle contents, gates
what produces alerts).

**Layer 2 — Tenant selection (deferred to R1.5+).** Each tenant
chooses which catalog categories / packs are active for their
discovery scans. Default for new tenants: all categories enabled. The
tenant's selection is carried in the discovery directive and
translated to nuclei `-tags` / `-itags` / `-it` flags at scan time.
Out of R1a scope; ships in a follow-on phase with:

- New `tenant_recon_config` JSONB on `tenants` (or its own table),
  shape `{enabled_categories: [...], severity_floor: "low",
  extra_tags: [...]}`.
- Backoffice "Catalog" surface (super-admin curates the available
  catalog, retires deprecated packs).
- Tenant settings page "Recon Templates" with category toggles and a
  severity-floor selector.
- Discovery directive extension (agent contract) — list of enabled
  tags/categories.

R1a ships with all-enabled defaults so the product is functional out
of the box; per-tenant selection becomes an upgrade, not a release
blocker.

## Schema summary

```
discovered_assets         — current-state inventory, one row per (tenant, ip, port)
asset_events              — append-only security-relevant changes
asset_sets                — D13: saved predicates over discovered_assets
targets                   — existing, refactored to reference assets
correlation_rules         — D2: generalized match → action engine
notification_channels     — D12: pluggable outbound channel configs
notification_deliveries   — D12: audit log of dispatches
one_shot_scans            — D13: parent record for fan-out scans
credential_sources        — see ADR 004
```

## Implementation phases

| Phase | Scope | Prereq |
|---|---|---|
| **R0** | Asset/target schema migration; `credential_source_id` on targets; `asset_sets`, `correlation_rules`, `notification_channels`, `notification_deliveries`, `one_shot_scans` tables | ADR 004 Phase C0 |
| **R1a** | L3 discovery (`target_type: network_range`); recon runner in agent; Assets page (read-only); manual/discovered unification; customer-controlled scan allowlist (D11); evidence redaction | R0 |
| **R1b** | Generalized D2 rule engine (`suggest_target` + `auto_create_target` actions); promote-to-compliance flow end-to-end | R1a |
| **R1c** | D12 notification channels (webhook, slack, email); D13 asset sets; `notify` and `run_one_shot_scan` rule actions; one-shot scan dispatcher | R1b |
| **R1.1** | Host list import target source; PagerDuty channel | R1c |
| **R1.5** | D14 tenant template selection: `tenant_recon_config`, backoffice catalog UI, tenant settings page, directive carries enabled tags | R1c |
| **R2** | AWS cloud discovery (`target_type: aws_account`); cloud-native credential binding; **orphaned-resource detection** (idea B — last_observed_traffic, tag-based filters on Assets page) | R1c, ADR 004 Phase C1 |
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
- **Evidence redaction** (audit 4.3, R1 implementation requirement).
  Nuclei templates frequently capture response bodies that may contain
  PII, session tokens, or other secrets. The recon runner must apply a
  redaction pass before persisting `scan_results.evidence` JSONB:
  - Mask common secret patterns (AWS keys, JWTs, bearer tokens,
    private keys, db connection strings) with a regex set.
  - Truncate captured response bodies to a configurable byte ceiling.
  - Customer-overridable redaction rules per tenant (config file or
    UI later).
  Goal: don't accidentally exfiltrate customer secrets into our DB
  while doing security work for them.
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
