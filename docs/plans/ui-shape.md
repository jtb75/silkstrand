# UI shape — asset-first

Companion to ADR 006 (asset-first data model) and the refactor roadmap.
This doc is the UI engineer's reference for two things:

1. The left-nav structure after the asset-first refactor.
2. The Asset detail view's section hierarchy and what each section shows.

The data model that backs this view is defined in ADR 006 and (pending) ADR
007. This doc does not introduce new tables — everything here either reads
existing/proposed columns or is a computed roll-up at the API layer.

## Motivation

Today the left nav carries eleven top-level items (Dashboard, Assets,
Targets, Agents, Scans, Asset Sets, Rules, Channels, One-shot, Team,
Settings). The flat shape is a direct reflection of the old mental model —
every feature got its own nav slot. Asset-first is a tighter story: the user
lands on assets, filters them into collections, and derives everything else
(scans, findings, coverage) from that. The nav should mirror that.

At the same time, the Assets detail drawer grew organically through R0 → R1c
and now renders a long flat list of facts. It needs a deliberate hierarchy:
what's stable, what changed, what's bad, what's configured.

## Nav structure

Four bands, ordered by access frequency. Members see the full nav minus the
`[admin]` items (matches today's pattern).

### Band 1 — Operator (daily)

**Dashboard.** Landing page. Widget board with per-collection counts, a risk
trend chart, new-this-week list, and recently-failed scans. Each widget is
backed by a Collection (`collections.is_dashboard_widget = true`). Empty
state when no widgets configured.

**Assets.** Primary inventory. Filter chips + predicate-builder above the
table; "Save as Collection" action on the chip bar turns the current filter
into a saved Collection. Row click opens the Asset detail view (spec in
§ "Asset detail view" below).

**Findings.** Cross-asset derived view. Two tabs:

- **Vulnerabilities** — network-sourced (nuclei today, httpx-based checks
  later). Filter by severity, source, collection, asset_endpoint.
- **Compliance** — both network-compliance (passive / unauthenticated) and
  bundle-compliance (authenticated deep scans), unified by a `source_kind`
  column on the findings table. Same filter set.

Findings is top-level, not nested under Assets, because the workflow is
different: SOC staff read findings daily; IT / asset-management staff live
on Assets. Cross-links keep them connected (row click on a finding → asset
detail view, filtered to the relevant endpoint and finding tab).

**Collections.** Saved predicates. Two tabs:

- **My collections** — every saved predicate the user has access to.
- **Dashboard widgets** — the subset where `is_dashboard_widget = true`,
  with per-widget config (title, widget_kind: count / table).

Inline create / edit / delete. Edit opens the PredicateBuilder (existing
component, reused).

**Scans.** Configured scan activity. Three tabs on one page:

- **Definitions** — list of scan_definitions. Shows `kind`, bundle or
  discovery, scope (asset_endpoint / collection / cidr), schedule, next run,
  last status. Actions: create new, edit, toggle enabled, run now. Includes
  manual-only (schedule=null) definitions — this is where one-shot scans
  live after the refactor.
- **Activity** — execution history from `scans`. Filter by definition,
  status, date range. Click a row → Scan Results (today's page, with the
  Findings tab added in Phase 4).
- **Targets** — CIDR / network-range scan scopes only. After Phase 7,
  nothing else. Sits under Scans because post-refactor a Target is "a scan
  scope that isn't a discovered asset."

### Band 2 — Automation (weekly)

**Rules** `[admin]`. Correlation rules list. Each rule shows name, trigger,
match-collection name, actions, enabled toggle, version. Edit opens a form;
save auto-versions. Delete soft-disables the latest version. No sub-nav.

### Band 3 — Infrastructure (monthly)

**Agents** `[admin]`. Fleet list with install flow (binary / docker
one-liner). Per-agent actions: allowlist viewer, upgrade, rotate key,
delete. Install-command generator includes the mode toggle
(binary / docker) from the recently-shipped installer changes.

### Band 4 — Setup

**Settings.** Sectioned page (tabs in the current UI pattern):

- **Profile** — display name, password change.
- **Team** `[admin]` — users, invitations, memberships. (Moved in from
  today's top-level Team nav.)
- **Credentials** — three sub-sections inside the tab:
  - *DB / host auth* — static-type credential sources used by compliance
    scans.
  - *Integrations* — Slack, webhook, email (planned), PagerDuty (planned).
    Absorbs today's Channels. These are credential_sources with the
    appropriate `type`.
  - *Vaults* — HashiCorp Vault, AWS Secrets Manager, CyberArk. Plumbing
    only until ADR 004 C1+ resolvers land.
- **Audit log** `[admin]` — ADR 005 surface when shipped. Read-only log.

### What goes away

- **Asset Sets** → folded into Collections.
- **Channels** → folded into Settings → Credentials → Integrations.
- **One-shot** → folded into Scans → Definitions (as manual-only
  definitions with a visual tag).
- **Team** → folded into Settings.

### What stays top-level (and why)

- **Rules.** Weekly workflow; full-list authoring surface; belongs in its
  own place, not buried in Settings.
- **Agents.** Install-token regeneration, allowlist troubleshooting, and
  upgrades are recurring fleet-ops tasks distinct from tenant settings.
- **Findings.** Primary SOC surface; needs its own entry point, not nested
  under Assets.

### Topbar (unchanged)

Tenant switcher (multi-tenant users), user email, log-out. No nav items
here; the topbar is identity-scope only.

## Asset detail view

The detail view opens as a drawer from the Assets table (today's
interaction) but is organized into explicit sections in the order a human
would read the page: *what is this → how long has it been here → should I
care → what's the detail → is it handled → what else does it touch*.

```
┌─ Asset ────────────────────────────────────────┐
│  [hostname or primary_ip]                      │
│  resource_type · environment · source          │
├────────────────────────────────────────────────┤
│  ❶  Identity                                   │
│       primary_ip, hostname                     │
│       resource_type (host/container/cloud)     │
│       fingerprint (expandable JSONB)           │
│       tags / owner / environment               │
│                                                │
│  ❷  Lifecycle                                  │
│       first_seen · last_seen · status          │
│         (live / stale / removed)               │
│       discovery provenance: target, agent,     │
│         scan_id that first surfaced this       │
│                                                │
│  ❸  Risk posture                               │
│       severity roll-up (critical/high/med/low) │
│       trend since last scan (Δ)                │
│       top 3 findings by severity (links)       │
│                                                │
│  ❹  Endpoints                                  │
│       table — one row per asset_endpoint       │
│         port / protocol / service / version    │
│         finding-count badge                    │
│         coverage icon (configured? creds?)     │
│       click row → expand inline:               │
│         · service + version + fingerprint      │
│         · findings (vuln + compliance tabs)    │
│         · credential binding                   │
│         · coverage (config, last/next scan,    │
│           last error)                          │
│                                                │
│  ❺  Coverage roll-up                           │
│       % endpoints with scan_definition         │
│       % endpoints with credential_mapping      │
│       last scan (most recent across ports)     │
│       next scan (earliest across defs)         │
│       configuration-gap list (endpoints with   │
│         no scan / no creds / recent failures)  │
│                                                │
│  ❻  Relationships                              │
│       depends-on / parent-of / peers           │
│       (empty until container + cloud ingest    │
│        land; placeholder section now)          │
└────────────────────────────────────────────────┘
```

### Data source for each section

| Section | Source |
|---|---|
| ❶ Identity | `assets` row (ADR 006 D2) |
| ❷ Lifecycle | `assets.first_seen` / `last_seen`; provenance from `asset_discovery_sources` (ADR 006 D9 — being added) |
| ❸ Risk posture | Computed roll-up at the API handler from `findings` (ADR 007) scoped to this asset's endpoints, with trend vs. previous scan |
| ❹ Endpoints | `asset_endpoints` (ADR 006 D2). Expand-on-click joins findings + credential_mappings + scan coverage |
| ❺ Coverage roll-up | Computed: `scan_definitions` LEFT JOIN `asset_endpoints`; `credential_mappings` LEFT JOIN endpoints (via collection membership); latest `scans` by endpoint |
| ❻ Relationships | Future `asset_relationships` table; section shows an "empty" state until populated |

Computed sections (❸, ❺) don't require new tables. The handler returns a
`coverage` and `risk` object alongside the flat asset view in the API
response. UI renders them without any additional round-trip.

### Interaction details

- **Endpoints table defaults to collapsed**, expand-on-click. If an asset
  has many endpoints (SMB server exposing 100+ ports) the drawer stays
  readable.
- **"Promote" action** (today's one-click Promote-from-Suggestion) hangs
  off the per-endpoint expanded row, not the top of the drawer. It applies
  to a specific port, which is what "promote" has always meant.
- **Risk posture badge** (❸) also appears in the Assets list row as a
  severity dot + count, so the table is scannable without opening the
  drawer.
- **Coverage gap list** (❺) is actionable: each row has a "Configure scan"
  or "Map credential" shortcut that opens the appropriate Settings flow
  pre-filled for that endpoint.

### Mobile / narrow-viewport

The drawer collapses the sections into a vertical accordion. Default open:
❸ Risk posture and ❹ Endpoints. Others collapsed by default. No feature
changes.

## Out of scope

- Visual design / typography — this doc defines structure, not style.
- Per-widget dashboard configuration UX — Phase 2 UI work; spec later.
- Rules editor layout changes — the existing form is fine; only the
  `match` selector changes to a collection dropdown.
- Reports / export — deferred. Collections + "export to CSV" on any list
  covers the near-term need.
