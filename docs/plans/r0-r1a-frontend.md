# R1a Frontend Plan — Assets Page + Interactive Topology

**ADR:** [003](../adr/003-recon-pipeline.md) — Phase R1a (tenant frontend slice only)
**Status:** Proposed
**Author:** planning pass, 2026-04-13
**Scope:** tenant web app only (`web/`). No backoffice, no API, no agent work.

---

## 1. Summary

R1a's frontend ships:

- A new `/assets` route — the centerpiece page per ADR 003 D3.
- List view with seven filter chips (D3): `With CVEs`, `Compliance candidates`,
  `Failing compliance`, `Recently changed`, `New this week`, `Manual`,
  `Discovered`.
- Asset detail drawer showing service, version, technologies, CVE list,
  compliance status, `asset_events` timeline (D4), and a D11 allowlist badge.
- Interactive topology view (React Flow / `@xyflow/react`) on a tab of the
  same route. Nodes = assets, grouping = auto `/24` by default, with
  environment and service-type as alternate grouping modes. Edges are
  placeholder (subnet-membership only) in v1.
- Manual target-creation path on the existing Targets page gets a short
  notice that an asset row is written first (D6).
- Targets page gets a one-line informational banner pointing at `/assets`
  (no functional changes — D3 migration is long-term).
- Live updates during running discovery scans via `refetchInterval`,
  mirroring `web/src/pages/Scans.tsx:18-26`.

Screen count: **4** (Assets list, Assets topology, Asset detail drawer,
Manual target create with new notice). tsc/eslint-clean will be enforced
before commit per user-memory.

---

## 2. Library Decision

**Ratified:** `@xyflow/react` (the maintained successor to the older
`react-flow-renderer`).

| Concern | Note |
|---|---|
| Bundle size | ~90 KB gzipped (xyflow core + react adapter). Acceptable — only loaded on the `/assets` route; lazy-load via `React.lazy` on the topology tab. |
| Peer deps | `react`, `react-dom` (already present at 19.2). No other peers. |
| License | MIT. |
| Maintenance | Actively released; successor namespace `@xyflow/*`. |
| API fit | Handles custom node components, grouping via parent nodes, background + controls + minimap, keyboard/pan/zoom. |

**Alternative considered:** `cytoscape.js` — heavier (~300 KB), better for
large graph algorithms (shortest-path, centrality), which R1a doesn't
need. Reject for v1.

**Alternative considered:** hand-rolled SVG — feasible for grouped static
layout but pan/zoom/drag/minimap is non-trivial. Reject; not worth the
saved bytes for a solo dev.

**Decision:** add `@xyflow/react` as a direct dependency in `web/package.json`.

---

## 3. Page Architecture

### Routes

Single route, tabbed view:

```
/assets            → Assets page, tab state in URL query (?tab=list|topology)
/assets/:id        → same page, opens drawer for that asset on mount
```

Rationale: keeps filter state, scroll, and scan-progress polling alive
when the user toggles between list and topology. URL-tab via query string
(`?tab=topology`) is cheap and bookmarkable. `/assets/:id` is optional
sugar so detail views can be deep-linked from the Dashboard or Scans
page later; the drawer closes back to whichever tab was active.

### Nav

Add **Assets** as the second item in the sidebar (after Dashboard,
before Targets) in `web/src/components/Layout.tsx`. Targets stays.

### Component tree

```
AssetsPage (pages/Assets.tsx)
├── AssetsTabBar
├── AssetsFilterChips
└── tab === 'list' ?
    ├── AssetsTable
    │   └── AssetRow × n  (inline, not its own file)
    └── AssetDetailDrawer  (portal; open when :id present)
    : tab === 'topology' ?
    └── AssetsTopology  (lazy-loaded)
        ├── GroupingControls
        └── ReactFlow canvas (custom AssetNode + GroupNode)
```

---

## 4. Mockups

### 4.1 Assets page — List tab

```
+------------------------------------------------------------------+
| Assets                                                [New Asset] |
+------------------------------------------------------------------+
| [ List ] [ Topology ]                                             |
+------------------------------------------------------------------+
| Filters: (With CVEs) (Compliance candidates) (Failing compliance)|
|          (Recently changed) (New this week) (Manual) (Discovered)|
|          Showing 142 of 142     [Clear]          scan running... |
+------------------------------------------------------------------+
| IP:Port        Host            Service   Ver    CVE  Comply  Src |
|------------------------------------------------------------------|
| 10.0.0.5:5432  db-prod-1       postgres  16.2   0    pass    M   |
| 10.0.0.7:443   -               nginx     1.24   2    -       D   |
| 10.0.0.9:8080  jenkins.corp    jenkins   2.4    1    fail    D   |
| 10.0.0.21:22   -               ssh       9.6    0    -       D   |
| 10.0.10.3:5984 couch-stg       couchdb   3.3    0    -       D   |
| ...                                                              |
+------------------------------------------------------------------+
```

### 4.2 Assets page — Topology tab

```
+------------------------------------------------------------------+
| Assets                                                            |
+------------------------------------------------------------------+
| [ List ] [*Topology*]     Group by: (/24) (env) (service)         |
+------------------------------------------------------------------+
|                                                                   |
|   +------------------+              +----------------------+      |
|   | 10.0.0.0/24  (4) |------edge----| 10.0.10.0/24  (112)  |      |
|   +------------------+   (subnet)   +----------------------+      |
|      |      |                             |                       |
|   [pg 5432][nginx 443]                [drill-in to expand]        |
|                                                                   |
|   +------------------+                                            |
|   | 10.50.0.0/24 (1) |   (DMZ)                                    |
|   +------------------+                                            |
|      |                                                            |
|   [jenkins 8080 !CVE]                                             |
|                                                                   |
|   [Controls: + - fit]  [Minimap]    cap: 300 nodes                |
+------------------------------------------------------------------+
```

### 4.3 Asset detail drawer

```
                       +------------------------------------------+
                       | Asset 10.0.0.9:8080                  [x] |
                       +------------------------------------------+
                       | jenkins.corp.local                       |
                       | service: jenkins    version: 2.4         |
                       | source: discovered  first: 2026-03-02    |
                       | scan policy: [allowlisted]     D11       |
                       +------------------------------------------+
                       | Technologies: jetty, java-11, jenkins    |
                       +------------------------------------------+
                       | CVEs (1):                                |
                       |   CVE-2024-43044  medium   jenkins-2.4   |
                       +------------------------------------------+
                       | Compliance: fail (cis-jenkins-2 bundle)  |
                       +------------------------------------------+
                       | Events:                                  |
                       |  2026-04-12  compliance_fail             |
                       |  2026-04-10  new_cve  CVE-2024-43044     |
                       |  2026-04-09  version_changed  2.3→2.4    |
                       |  2026-03-02  new_asset                   |
                       +------------------------------------------+
```

### 4.4 Manual target create form (updated notice)

```
+------------------------------------------------------------------+
| New Target                                                        |
+------------------------------------------------------------------+
| Note: creating a target first registers an asset in your         |
| inventory (source=manual). Future discovery passes will enrich   |
| the same row in place.                                           |
+------------------------------------------------------------------+
| Technology: [ PostgreSQL       v ]                               |
| Name:       [ studio-prod-postgres                ]              |
| Agent:      [ agent-nyc-1 (online)           v ]                 |
| Host:       [ 10.0.0.5              ] Port: [ 5432 ]             |
| Database:   [ postgres      ]  SSL: [ prefer v ]                 |
|                                                                   |
|                                          [Cancel] [Create Target]|
+------------------------------------------------------------------+
```

---

## 5. Component Breakdown

One file per row, under `web/src/`. Target ≤10 components; this is 8.

| File | Props | Renders |
|---|---|---|
| `pages/Assets.tsx` | – (reads route + query) | Page shell: header, tab bar, filter chips, active-tab body, drawer portal. Owns `tab` state via `useSearchParams` and `selectedAssetId` via `useParams`. |
| `components/AssetsTabBar.tsx` | `tab: 'list'\|'topology'; onChange(tab)` | Two buttons; active styling. |
| `components/AssetsFilterChips.tsx` | `active: Set<ChipId>; counts: Record<ChipId, number>; onToggle(id)` | Renders 7 toggleable chips + `Clear` + live count summary + "scan running" indicator. |
| `components/AssetsTable.tsx` | `assets: Asset[]; onSelect(id)` | Table of asset rows. Handles click → drawer open via nav. |
| `components/AssetDetailDrawer.tsx` | `assetId: string; onClose()` | Slide-in panel (CSS `transform`), fetches `['asset', id]`, renders sections from mockup 4.3. |
| `components/AssetEventTimeline.tsx` | `events: AssetEvent[]` | Vertical list of events with icon per `event_type`, relative timestamp. Pure, used inside drawer. |
| `components/AssetsTopology.tsx` | `assets: Asset[]; grouping: 'cidr24'\|'env'\|'service'` | React Flow canvas. Builds nodes/edges from props. Lazy-loaded via `React.lazy` from Assets.tsx. |
| `components/AllowlistBadge.tsx` | `status: 'allowlisted'\|'out-of-policy'\|'unknown'` | Small colored pill used in both table and drawer. |

Reused existing files: `Layout.tsx` (nav link), `Targets.tsx` (notice
line + banner), `api/client.ts` + `api/types.ts` (new functions/types).

---

## 6. Data Fetching

### Query keys

```
['assets', { filters, page }]   → GET /api/v1/assets?filter=...&page=...
['asset', id]                   → GET /api/v1/assets/:id  (includes events)
['scans']                       → existing; used to gate polling
```

### Refetch policy

`['assets', ...]` uses the Scans pattern:

```ts
refetchInterval: () => {
  const scans = queryClient.getQueryData<Scan[]>(['scans']);
  if (scans?.some((s) => s.type === 'discovery' &&
                         (s.status === 'running' || s.status === 'pending'))) {
    return 5000;
  }
  return false;
},
refetchIntervalInBackground: false,  // pause when tab hidden
```

We must also invalidate `['scans']` at the same 5s cadence so the gate
stays fresh — piggyback off the existing Scans polling; `['assets']`
reads the cache. React Query's default `refetchOnWindowFocus: false`
(set in `App.tsx:20`) is respected.

`['asset', id]` fetches on drawer-open, revalidates on window focus
disabled; optional re-fetch when a polling tick updates `['assets']`
list → invalidate just the open asset's cache key.

### Optimistic updates

None in R1a — the entire surface is read-only. Manual target-create
still flows through the existing `createTarget` mutation; after that
mutation succeeds we additionally invalidate `['assets', ...]` so the
new `source=manual` asset row appears without a reload.

### Filter threshold

Client-side filter chips against the fetched list while total count ≤ 500.
Above that, chip activation issues a server query with `?filter=<csv>`.
Threshold baked in as a constant in `Assets.tsx`; start at 500, tune.

---

## 7. API Contract Dependencies

The API planner must provide (shapes TBD in the API plan):

- `GET /api/v1/assets` — list with server-side filters matching the seven
  chip IDs (`with_cves`, `compliance_candidate`, `failing`, `recently_changed`,
  `new_this_week`, `manual`, `discovered`), pagination (`page`, `page_size`),
  and a total count. Response rows must carry enough fields to render
  mockup 4.1 (ip, port, hostname, service, version, cve_count,
  compliance_status, source, first_seen, last_seen, allowlist_status).
- `GET /api/v1/assets/:id` — single asset plus its `asset_events` (last N).
- `GET /api/v1/scans` — **must include `type: 'discovery' | 'compliance'`**
  on each row so the poll-gate in §6 can recognize discovery-scan progress.
  If the field doesn't exist today, the API planner owns adding it.
- Agent allowlist read exposure (D11): each asset row must carry a derived
  `allowlist_status`. API computes this by comparing the asset's ip
  against the last reported allowlist snapshot from the owning agent.
  If that snapshot isn't yet plumbed from agent → API, the badge falls
  back to `unknown` and the UI renders gracefully.
- Existing `POST /api/v1/targets` keeps its shape; API must guarantee the
  D6 behavior that target creation upserts `discovered_assets` first.
  The frontend displays the notice unconditionally; correctness is a
  backend concern.

---

## 8. Open Items / Cross-Team Dependencies

**From the API planner:**

1. Confirm the `scans.type` field or propose an alternative gate
   signal for the poll.
2. Decide whether `GET /api/v1/assets` returns `allowlist_status` inline
   or requires a second call — prefer inline.
3. Confirm filter-parameter encoding (`?filter=with_cves,manual` vs
   repeated params vs a JSON body on a POST-search). Prefer CSV on GET.
4. Event-history cap on `GET /api/v1/assets/:id` — propose 50 most
   recent; UI will paginate later if needed (out of R1a scope).

**From the data-model planner:**

1. Confirm `discovered_assets` column set exactly matches what §7 asks
   of the API (esp. `compliance_status`, `source`, `first_seen`,
   `last_seen`).
2. Confirm `asset_events` carries `event_type` exactly as enumerated in
   ADR 003 D4 so the timeline component can switch on a closed set.
3. Confirm the targets table migration (D6) keeps `targets.identifier`,
   `targets.environment`, `targets.agent_id` readable by the existing
   Targets page without schema rewrite.

**Open UX questions (decide during implementation):**

- Topology: do we also offer a plain HTML "grouped list" fallback for
  screens where React Flow performs poorly or users prefer text?
  Proposal: out of R1a; revisit if a customer complains.
- Targets page banner wording — confirm copy with product once the
  long-term `/assets` story is articulated externally.

---

## Commit hygiene

Per user-memory: `npx tsc --noEmit` and `npx eslint .` run cleanly before
any commit touching TS/React files. This plan's implementation will
respect that gate.
