# UI shape — asset-first (v1.1)

Companion to ADR 006 (asset-first data model) and ADR 007 (findings +
scheduler). This doc is the UI engineer's reference for:

1. The global layout and spacing system.
2. The left-nav structure after the asset-first refactor.
3. The page-by-page layout for Dashboard, Assets, Collections, Scans.
4. The Asset detail view's section hierarchy.
5. The primary operator workflow that ties them together.

The data model that backs this view is defined in ADR 006 and ADR 007. This
doc does not introduce new tables — everything here either reads
existing/proposed columns or is a computed roll-up at the API layer.

## Motivation

Today the left nav carries eleven top-level items and the Assets detail
drawer renders a long flat list of facts. Asset-first is a tighter story:
the user lands on assets, filters them into collections, and derives
everything else (scans, findings, coverage) from there. The nav and page
shapes should mirror that.

## Guiding principles

- **Endpoints are the operational surface.** Most work (assigning credentials,
  configuring scans, reading findings) happens at the endpoint level.
- **Collections are the control plane.** Filter once, save as a collection,
  reuse in dashboards, rules, and scans.
- **Coverage is visible everywhere.** Scan coverage and credential coverage
  are first-class signals on every asset/endpoint row.
- **Bulk actions are first-class.** Multi-select + apply is the common case,
  not the exception.
- **The Dashboard drives action, not just visibility.** Suggested Actions is
  the single source of truth for coverage gaps — no duplicated "assets
  without scans" widget.

## Global layout

```
┌────────────────────────────────────────────────────────────────────────────┐
│ Topbar (56px)                                                              │
├───────────────┬────────────────────────────────────────────────────────────┤
│ Sidebar 220px │ Content Area (max-width: 1200px, left-aligned)             │
│               │ padding: 24px                                              │
└───────────────┴────────────────────────────────────────────────────────────┘
```

**Spacing system**

- Page padding: 24px
- Section spacing: 32px
- Card padding: 16–24px
- Grid gap: 16px
- Table row height: 42px

## Nav structure

Four bands, ordered by access frequency. Members see the full nav minus the
`[admin]` items (matches today's pattern).

### Band 1 — Operator (daily)

- **Dashboard** — landing page. KPI row + widget grid + Suggested Actions
  (see § Dashboard).
- **Assets** — primary inventory. Three tabs: Assets / Endpoints / Findings
  (same filtered population, three views). Bulk Actions bar (see § Assets).
- **Findings** — top-level SOC view across all assets. Two tabs:
  Vulnerabilities, Compliance. Top-level because the workflow audience (SOC)
  differs from Assets' audience (IT / asset-management). Cross-links:
  click-through on a finding pops the Asset detail drawer scoped to the
  relevant endpoint and finding tab.
- **Collections** — saved predicates. Scope covers assets, endpoints, and
  findings (see § Collections). Two tabs: My Collections, Dashboard Widgets.
- **Scans** — configured scan activity. Three tabs: Definitions, Activity,
  Targets. Targets narrows to CIDR / network-range after the refactor.
  Coverage Impact strip on the Definitions list shows how many endpoints
  each definition hits.

### Band 2 — Automation (weekly)

- **Rules** `[admin]`. Correlation-rule list. Rule match references a
  collection_id (ADR 006 D6). Inline enable/disable; edit auto-versions.

### Band 3 — Infrastructure (monthly)

- **Agents** `[admin]`. Fleet list + install flow (binary / docker). Per-agent:
  allowlist viewer, upgrade, rotate key, delete.

### Band 4 — Setup

- **Settings**. Sectioned page:
  - **Profile** — display name, password change.
  - **Team** `[admin]` — users, invitations, memberships.
  - **Credentials**
    - *DB / host auth* — static-type credential sources.
    - *Integrations* — Slack, webhook, email, PagerDuty. Absorbs today's
      Channels. Stored as `credential_sources` with the appropriate `type`.
    - *Vaults* — HashiCorp, AWS SM, CyberArk. Plumbing only until ADR 004
      C1+ resolvers land.
  - **Audit log** `[admin]` — ADR 005 surface. Read-only.

### What goes away

- **Asset Sets** → Collections.
- **Channels** → Settings → Credentials → Integrations.
- **One-shot** → Scans → Definitions (manual-only, visual tag).
- **Team** → Settings.

### What stays top-level (and why)

- **Rules** — full-list authoring surface; weekly workflow.
- **Agents** — fleet ops are distinct from tenant settings.
- **Findings** — primary SOC surface; needs its own entry point.

### Topbar

Tenant switcher, user email, log-out. Identity-scope only.

## Dashboard

**Design decision.** Coverage gaps are surfaced ONLY in Suggested Actions.
No duplicated "assets without scans" widget. Suggested Actions is the single
source of truth for "here's what needs attention."

### Layout

- Header with page title + `[ + New Scan ]`.
- **KPI row (4 cards)** — Total Assets, Coverage %, Critical Findings, New
  This Week. Each card shows the metric + a delta ("+12 this wk").
- **Main grid** — 8-col left, 4-col right.
  - Left: Unclassified Endpoints list (collection-backed), Activity stream
    (collections or events).
  - Right: Suggested Actions, Recent Activity.

### ASCII reference

```
┌────────────────────────────────────────────────────────────────────────────┐
│ SilkStrand                                  [Tenant ▼] [User] [⚙]         │
├───────────────┬────────────────────────────────────────────────────────────┤
│ Dashboard     │  DASHBOARD                                                 │
│ Assets        │                                                            │
│ Findings      │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐        │
│ Collections   │  │ Total Assets │ │ Coverage     │ │ Critical     │        │
│ Scans         │  │ 128          │ │ 62%          │ │ 7            │        │
│────────────── │  │ +12 this wk  │ │ ███████░░░░  │ │ +2 today     │        │
│ Rules         │  └──────────────┘ └──────────────┘ └──────────────┘        │
│ Agents        │  ┌──────────────┐                                          │
│────────────── │  │ New This Week│                                          │
│ Settings      │  │ 12           │                                          │
│               │  │ +3 unresolved│                                          │
│               │  └──────────────┘                                          │
│               │  ┌──────────────────────────────────────────────────────┐  │
│               │  │ Unclassified Endpoints (12)                          │  │
│               │  │──────────────────────────────────────────────────────│  │
│               │  │ 10.0.0.5:5432   postgres   ❌ no scan                │  │
│               │  │ 10.0.0.8:443    nginx      ⚠ 2 findings             │  │
│               │  │ 10.0.0.12:22    ssh        ❌ no creds               │  │
│               │  │                                              [View]  │  │
│               │  └──────────────────────────────────────────────────────┘  │
│               │                              ┌───────────────────────────┐ │
│               │                              │ Suggested Actions         │ │
│               │                              │───────────────────────────│ │
│               │                              │ 12 DB endpoints missing   │ │
│               │                              │ credentials               │ │
│               │                              │ [ Map Credentials ]       │ │
│               │                              │ [ Create Scan ]           │ │
│               │                              └───────────────────────────┘ │
│               │                              ┌───────────────────────────┐ │
│               │                              │ Recent Activity           │ │
│               │                              │───────────────────────────│ │
│               │                              │ + Asset discovered        │ │
│               │                              │ ✔ Scan completed          │ │
│               │                              │ ⚠ Scan failed             │ │
│               │                              └───────────────────────────┘ │
└───────────────┴────────────────────────────────────────────────────────────┘
```

### Widget data sources

| Widget | Source |
|---|---|
| KPI cards | Aggregate queries on assets / findings / scan_definitions |
| Unclassified Endpoints | Collection with `scope = endpoint` filtering to `service = NULL OR service = 'unknown'` |
| Suggested Actions | Computed server-side: groups of coverage gaps (no scan / no creds / failing), with a primary CTA per group |
| Recent Activity | `asset_events` feed (last 10) or audit-events feed once ADR 005 ships |

## Assets

**Three tabs, one filtered population.** Assets / Endpoints / Findings tabs
on the Assets page show the same filter state rendered differently:

- **Assets tab** — one row per asset (host-level). Columns: Host, IP,
  resource_type, env, #endpoints, max-severity-finding, coverage pair
  (scan / creds).
- **Endpoints tab** — one row per asset_endpoint. Columns: Host, IP:Port,
  Service, Tech, Findings count, Coverage pair.
- **Findings tab** — one row per finding, scoped to the filtered population.
  Columns: severity, title, source, asset:port, status, last seen.

The same filter bar (search + collection picker + "Save as Collection")
drives all three tabs. Multi-select persists across tabs.

### Bulk Actions bar

Row multi-select enables a persistent bottom bar with the primary actions.
v1 actions:

- **Map Credentials** — pick a credential_source, applied to every selected
  endpoint's collection (creates `credential_mappings` rows).
- **Create Scan** — open the New Scan Definition flow pre-filled with the
  selected endpoints as the scope.

Future: Add to Collection, Suppress findings, Assign owner.

### Coverage column

Two icons per row:

- **Scan** — ✔ if any scan_definition covers this endpoint; ❌ otherwise.
- **Creds** — ✔ if a credential_mapping resolves for this endpoint; ❌
  otherwise.

Also appears on the Dashboard's Unclassified Endpoints list and on the
endpoint rows inside the Asset detail drawer.

### ASCII reference

```
┌────────────────────────────────────────────────────────────────────────────┐
│ Assets                                                     [ + Filter ]    │
├────────────────────────────────────────────────────────────────────────────┤
│ [ Search............. ] [ Collection ▼ ] [ Save as Collection ]            │
│                                                                            │
│ Tabs:   Assets | ENDPOINTS | Findings                                      │
│                                                                            │
│ ┌────────────────────────────────────────────────────────────────────────┐ │
│ │ Host        IP         Port  Service     Tech        Findings  Coverage│ │
│ │────────────────────────────────────────────────────────────────────────│ │
│ │ db-01       10.0.0.5   5432  postgres    PG 14       ⚠ 3       ❌ ❌   │ │
│ │ web-01      10.0.0.8   443   https       nginx       ⚠ 2       ✔ ❌   │ │
│ │ admin-01    10.0.0.12  22    ssh         openssh     ✔ 0       ❌ ❌   │ │
│ └────────────────────────────────────────────────────────────────────────┘ │
│                                                                            │
│ Bulk Actions: [ Map Credentials ] [ Create Scan ]                          │
└────────────────────────────────────────────────────────────────────────────┘
```

## Collections

**Scope extends beyond assets/endpoints.** In addition to ADR 006 D5's
`scope = 'asset' | 'endpoint'`, Collections also accept `scope = 'finding'`
so operators can save reusable queries like "all open critical findings
this week" or "all compliance findings on prod databases" and reference
them from dashboards and rules. This is a one-line change to the enum
(tracked as an amendment to ADR 006 D5).

### Query Preview

Collections list inline-expands a plain-language rendering of the stored
predicate:

```
type = database AND scan_configured = false
```

Keeps the author honest about what the collection really selects without
forcing them into the JSONB view.

### ASCII reference

```
┌────────────────────────────────────────────────────────────────────────────┐
│ Collections                                                [ + New ]       │
├────────────────────────────────────────────────────────────────────────────┤
│ Tabs: My Collections | Dashboard Widgets                                   │
│                                                                            │
│ Name                         Type      Count   Last Updated   Actions      │
│────────────────────────────────────────────────────────────────────────────│
│ Unclassified Endpoints       Endpoint   12     2m ago        [Open]        │
│ DB without scans             Endpoint   18     10m ago       [Edit]        │
│ Critical findings            Finding    7      5m ago        [Open]        │
│────────────────────────────────────────────────────────────────────────────│
│ Query Preview:                                                             │
│ type = database AND scan_configured = false                                │
└────────────────────────────────────────────────────────────────────────────┘
```

## Scans

**Three tabs:** Definitions, Activity, Targets.

### Definitions tab

Columns: Name, Type, Scope, Schedule, Last, Next, Status. Scope renders as
`Collection:<name>`, `Endpoint:<ip:port>`, or `CIDR:<range>` depending on
`scan_definitions.scope_kind`.

### Coverage Impact strip

Below the definitions table, a rolling "who hits what" summary:

```
Coverage Impact:
- PG CIS covers 12 endpoints
- Network covers 128 IPs
```

Makes it immediately obvious whether a definition is doing any real work.
Answered at author time from the collection membership evaluation (ADR 007
D4 — re-resolved per tick, cached for the preview).

### ASCII reference

```
┌────────────────────────────────────────────────────────────────────────────┐
│ Scans                                                      [ + New Scan ]  │
├────────────────────────────────────────────────────────────────────────────┤
│ Tabs: DEFINITIONS | Activity | Targets                                     │
│                                                                            │
│ Name        Type       Scope           Schedule   Last   Next   Status     │
│────────────────────────────────────────────────────────────────────────────│
│ PG CIS      compliance DB Collection   daily      OK     2h     ✔          │
│ Network     discovery  CIDR 10.0.0.0   24h        OK     4h     ✔          │
│ Ad-hoc DB   compliance manual         —           —      —      —          │
│                                                                            │
│ Coverage Impact:                                                           │
│ - PG CIS covers 12 endpoints                                               │
└────────────────────────────────────────────────────────────────────────────┘
```

## Asset detail view

The detail view opens as a drawer from the Assets table, organized into
explicit sections in the order a human would read the page: *what is this
→ how long has it been here → should I care → what's the detail → is it
handled → what else does it touch*.

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
│       [ Map Credential ] [ Create Scan ]       │
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
| ❷ Lifecycle | `assets.first_seen` / `last_seen`; provenance from `asset_discovery_sources` (ADR 006 D9) |
| ❸ Risk posture | Computed at the API handler from `findings` (ADR 007 D1) scoped to this asset's endpoints, with trend vs. previous scan |
| ❹ Endpoints | `asset_endpoints` (ADR 006 D2). Expand-on-click joins findings + credential_mappings + scan coverage |
| ❺ Coverage roll-up | Computed: `scan_definitions` LEFT JOIN `asset_endpoints`; `credential_mappings` LEFT JOIN endpoints (via collection membership); latest `scans` by endpoint |
| ❻ Relationships | Future `asset_relationships` table; empty state until populated |

Computed sections (❸, ❺) don't require new tables. The handler returns
`risk` and `coverage` objects alongside the flat asset view in one API
response. UI renders them without an extra round-trip.

### Interaction details

- **Endpoints table defaults to collapsed**, expand-on-click. Assets with
  many ports stay readable.
- **"Promote" action** hangs off the per-endpoint expanded row (it applies
  to a specific port, which is what promote has always meant).
- **Risk posture badge** (❸) also appears in the Assets list row so the
  table is scannable without opening the drawer.
- **Coverage gap list** (❺) rows each have "Configure scan" / "Map
  credential" shortcuts pre-filled for that endpoint.

### Mobile / narrow viewport

Sections collapse into a vertical accordion. Default open: ❸ Risk posture
and ❹ Endpoints. Others collapsed by default.

## Operator workflow (E2E)

The primary workflow the UI is shaped around:

```
Dashboard
   │  click: "12 DB endpoints missing credentials"  (Suggested Actions)
   ▼
Assets (Endpoints tab, filtered to the suggested collection)
   │  multi-select rows
   ▼
[ Map Credentials ]  ──►  pick credential_source, apply to selection
   │
   ▼
[ Create Scan ]  ──►  new scan_definition pre-filled with the selected
   │                   endpoints as scope; schedule daily
   ▼
Scans → Definitions  (new row visible, Coverage Impact updates)
   │
   ▼
Scans → Activity  (run materializes per the schedule or immediately via
                   "Run now")
```

Every arrow in this flow is a single click and stays inside the same
filtered selection. No re-filtering, no re-selecting, no page context loss.

## Out of scope

- Visual design / typography — structure only.
- Per-widget dashboard configuration UX — Phase 2 follow-on.
- Rules editor layout changes — the existing form stays; only the `match`
  selector changes to a collection dropdown.
- Reports / export — deferred. Collections + "export to CSV" on any list
  covers the near-term need.
