# R0–R1a API Server + Ingestion Plan (ADR 003)

## 1. Summary

R1a adds the read-only inventory slice to the API. Net-new surfaces:

- A `discovery` scan kind that reuses the existing `scans` table (D1 decision below), dispatched via the existing `POST /api/v1/scans` → Redis → `forwardDirective` → agent WSS pipeline.
- Two new agent→server WSS message types: `asset_discovered` and `discovery_completed`. `scan_error` is reused unchanged.
- A server-side ingestion path that upserts `discovered_assets` and appends `asset_events` on every `asset_discovered` message, streaming (no buffering until scan end).
- Read endpoints for the Assets page: `GET /api/v1/assets`, `GET /api/v1/assets/{id}`.
- `POST /api/v1/targets` gains a side-effect: create/locate a `discovered_assets` row with `source='manual'` inside the same transaction, then write `targets.asset_id` (D6).

Agent contract changes: directive gains a `scan_type` + discovery-shaped `target_config`; agent sends `asset_discovered` incrementally (D9).

## 2. Directive shape

Reuse `DirectivePayload` with one added field. `target_type` stays `network_range`; `target_identifier` carries the CIDR/range/IP/hostname literal; `target_config` carries discovery knobs.

```go
// api/internal/websocket/messages.go
type DirectivePayload struct {
    ScanID           string          `json:"scan_id"`
    ScanType         string          `json:"scan_type"`        // NEW: "compliance" | "discovery"
    BundleID         string          `json:"bundle_id"`
    BundleName       string          `json:"bundle_name"`
    BundleVersion    string          `json:"bundle_version"`
    BundleURL        string          `json:"bundle_url,omitempty"`
    TargetID         string          `json:"target_id"`
    TargetType       string          `json:"target_type"`      // "network_range"
    TargetIdentifier string          `json:"target_identifier"`// "10.0.0.0/24"
    TargetConfig     json.RawMessage `json:"target_config"`    // DiscoveryConfig for discovery scans
    Credentials      json.RawMessage `json:"credentials,omitempty"` // nil for discovery
}

type DiscoveryConfig struct {
    Ports            string `json:"ports,omitempty"`            // e.g. "top-1000" or "1-65535"
    RatePPS          int    `json:"rate_pps,omitempty"`         // soft ceiling; allowlist cap wins
    IncludeHTTPX     bool   `json:"include_httpx"`
    IncludeNuclei    bool   `json:"include_nuclei"`
    BatchSize        int    `json:"batch_size,omitempty"`       // assets per asset_discovered msg
}
```

## 3. WSS message types (agent → server)

```go
const (
    TypeAssetDiscovered    = "asset_discovered"
    TypeDiscoveryCompleted = "discovery_completed"
)

type AssetDiscoveredPayload struct {
    ScanID string             `json:"scan_id"`
    Assets []DiscoveredAsset  `json:"assets"` // batch (1..N), per D9 open item
}

type DiscoveredAsset struct {
    IP           string          `json:"ip"`
    Port         int             `json:"port"`
    Hostname     string          `json:"hostname,omitempty"`
    Service      string          `json:"service,omitempty"`
    Version      string          `json:"version,omitempty"`
    Technologies json.RawMessage `json:"technologies,omitempty"` // JSONB passthrough
    CVEs         json.RawMessage `json:"cves,omitempty"`         // [{id,severity,...}]
    ObservedAt   time.Time       `json:"observed_at"`
}

type DiscoveryCompletedPayload struct {
    ScanID       string `json:"scan_id"`
    AssetsFound  int    `json:"assets_found"`
    HostsScanned int    `json:"hosts_scanned"`
}
```

## 4. Handler-by-handler changes

### 4.1 `api/internal/handler/scan.go` — `Create`
- Parse `scan_type` from `CreateScanRequest` (add field; default `"compliance"` for back-compat).
- For `scan_type=="discovery"`: require `target_id` referencing a `network_range` target; `bundle_id` must be recon framework (`framework='recon-pipeline'`). No credentials lookup.
- Store `scan_type` on the scans row. Same `pending` status, same Redis dispatch path.

### 4.2 `api/internal/handler/agent.go` — `forwardDirective`
- Load the scans row (already implicit via Redis `Directive`, but we need `scan_type`). Add `ScanType` to `pubsub.Directive` so we avoid a re-fetch.
- Skip credential resolution when `scan_type=="discovery"`.
- Pass `ScanType` through `NewDirectiveMessage` (signature extended).

### 4.3 `api/cmd/silkstrand-api/main.go` — `buildOnMessage`
New cases wired to a new `handler/discovery_ingest.go` (so `main.go` stays a thin switch):

```go
case websocket.TypeAssetDiscovered:
    discoveryIngest.HandleAssetDiscovered(ctx, agentID, msg.Payload)
case websocket.TypeDiscoveryCompleted:
    discoveryIngest.HandleDiscoveryCompleted(ctx, agentID, msg.Payload)
```

### 4.4 NEW `api/internal/handler/discovery_ingest.go`
`DiscoveryIngestHandler` owns:
- `HandleAssetDiscovered`: load scan, tenant-context-ify, transition `pending→running` on first message (idempotent `UpdateScanStatusIfPending`), loop assets → `UpsertDiscoveredAsset` which returns `(assetID, []EventDraft)` → `AppendAssetEvents`. Inline in read loop (see §6 / Q6).
- `HandleDiscoveryCompleted`: `UpdateScanStatus` → `completed`, stamp `completed_at`.

### 4.5 `api/internal/handler/target.go` — `Create`
- When `type=="network_range"`: within one tx (new `store.CreateTargetWithAsset`), upsert a `discovered_assets` row (`source='manual'`, ip=identifier or resolved-at-create), then insert target with `asset_id` set.
- For not-yet-resolvable hostnames, asset row carries hostname + `ip=0.0.0.0`/nullable per data-model plan; coordinate with that planner.

### 4.6 NEW `api/internal/handler/asset.go`
`AssetHandler` with:
- `List(w,r)` — `GET /api/v1/assets`, paginated filtered.
- `Get(w,r)` — `GET /api/v1/assets/{id}`, includes recent events (default last 50; query `?events=200`).

### 4.7 `api/internal/pubsub/redis.go` — `Directive`
Add `ScanType string` to Directive struct (Redis JSON payload; back-compat default empty → `"compliance"`).

### 4.8 Stuck-scan cleanup
`FailRunningScansForAgent` already fails any `running` scan on disconnect — no change; discovery inherits it for free.

## 5. Store layer additions (signatures only)

```go
// Discovery scans share the scans table.
CreateDiscoveryScan(ctx context.Context, req model.CreateScanRequest) (*model.Scan, error) // or reuse CreateScan with scan_type
UpdateScanStatusIfPending(ctx context.Context, id, newStatus string) (bool, error)

// Asset ingestion
UpsertDiscoveredAsset(ctx context.Context, scanID string, a model.DiscoveredAssetInput) (
    asset *model.DiscoveredAsset, events []model.AssetEventDraft, err error)
AppendAssetEvents(ctx context.Context, events []model.AssetEventDraft) error

// Manual target unification
CreateTargetWithAsset(ctx context.Context, req model.CreateTargetRequest) (*model.Target, error)

// Read APIs
ListAssets(ctx context.Context, f model.AssetFilter) (items []model.DiscoveredAsset, total int, err error)
GetAsset(ctx context.Context, id string) (*model.DiscoveredAsset, error)
ListAssetEvents(ctx context.Context, assetID string, limit int) ([]model.AssetEvent, error)
```

`model.AssetFilter` carries the parsed query (see §6).

## 6. Asset filter API

### Endpoint
`GET /api/v1/assets?service=postgresql&cve_count_gte=1&new_since=7d&page=1&page_size=50&sort=last_seen:desc`

### Query params → predicate grammar
Pre-canned, flat key=value pairs on the wire; server translates into the structured JSONB predicate grammar shared with D2 rules / D13 asset sets. We deliberately do **not** expose the full DSL via query string in R1a (recommendation: full DSL can come later as `POST /api/v1/assets/search` with JSON body).

| Query param | Predicate |
|---|---|
| `service=postgresql` | `{service: "postgresql"}` |
| `service_in=postgresql,mysql` | `{service: {in: [...]}}` |
| `version=16.*` | `{version: {like: "16.*"}}` |
| `ip=10.0.0.0/16` | `{ip: {cidr: "10.0.0.0/16"}}` |
| `port=5432` / `port_in=...` | `{port: ...}` |
| `cve_count_gte=1` | `{cves: {len_gte: 1}}` |
| `has_cve=CVE-2024-4317` | `{cves: {contains_id: "..."}}` |
| `source=manual\|discovered` | `{source: "..."}` |
| `compliance_status=fail` | `{compliance_status: "fail"}` |
| `new_since=7d` | `{first_seen: {gte: now-7d}}` |
| `changed_since=7d` | `{last_seen: {gte: now-7d}}` |
| `q=<free text>` | ilike across hostname, service, version, ip::text |

Multiple params AND together; repeat-key semantics reserved.

### Pagination / sorting
- `page` (1-based), `page_size` (default 50, max 200).
- Envelope: `{"items":[...], "page":1, "page_size":50, "total":142}`.
- `sort` in `{last_seen|first_seen|ip|service|cve_count}:{asc|desc}`; default `last_seen:desc`.

### Filter chips
Frontend chips map to canned param-sets: `With CVEs` → `cve_count_gte=1`; `New this week` → `new_since=7d`; `Compliance candidates` → `service_in=postgresql,mysql,mssql,...`; `Failing compliance` → `compliance_status=fail`.

## 7. Design-question resolutions

- **Q1 (scans table reuse):** reuse. Add `scan_type TEXT NOT NULL DEFAULT 'compliance'`. Same lifecycle, same agent dispatch, same disconnect cleanup. Discovery has no `scan_results` rows — fine, separate result surface (`discovered_assets` + `asset_events`) is keyed on `scan_id` via `last_scan_id` and `asset_events.scan_id`. Cost of separation > benefit for one diff column.
- **Q2 (idempotency):**
  - Same (tenant,ip,port) twice in one scan: upsert is idempotent; second call sees `last_scan_id == scan_id && last_seen == now-ish && no attribute delta` → no event emitted.
  - Across overlapping scans: upsert unconditionally bumps `last_seen` and `last_scan_id`; attribute deltas emit events once per change.
  - Missing rows on re-scan (fewer services): **leave in place** in R1a. `asset_gone` is explicitly *not* emitted here; defer to the data-model plan / R1b — a post-scan reaper comparing `last_seen < scan.started_at` for CIDRs the scan owned is the cheap future option. Flag this as an open cross-team item.
- **Q3 (event derivation):** decision table applied per upsert (old vs new row):

| Condition | Event |
|---|---|
| `old IS NULL` | `new_asset` (payload: service, version, port, cve list) |
| `old.service != new.service` OR `old.version != new.version` | `version_changed` (payload: from/to) |
| `old IS NULL` AND port-level (first row for this ip) — covered by `new_asset`; if prior asset rows exist for the same IP on other ports and this (ip,port) is new | `port_opened` |
| `new.cves` has CVE id not in `old.cves` | one `new_cve` per added id |
| `old.cves` has CVE id not in `new.cves` | one `cve_resolved` per removed id |

`compliance_pass/fail` not emitted here (R1b). `port_closed` / `asset_gone` deferred.

- **Q4 (filter API shape):** see §6 — canned query params on the GET, JSON DSL later via POST.
- **Q5 (D11 API-side validation):** **no.** Allowlist is purely agent-side (two-key principle in ADR). API-side validation would give false security (SaaS compromise could lie about the check). Log the directive CIDR for audit; trust the agent to enforce. Frontend may show a warning comparing to a cached, agent-reported copy of the allowlist for UX, but that's advisory.
- **Q6 (backpressure):** process **inline** in `hub.OnMessage` for R1a. Rationale: solo-dev scope, no new abstractions, WSS read is already per-agent-serialized, Postgres upsert on `(tenant,ip,port)` is fast. Pressure valves: agent-side `batch_size` (start 10; tune via open item) and a hard WSS `maxMessageSize=1MB`. If a /16 pilot shows the read loop wedging, switch to an in-process buffered channel + single goroutine drain per agent — no Redis list, no new infra. Defer that until measured.

## 8. Open items / cross-team dependencies

**From data-model planner (sibling):**
- Final `scans.scan_type` column name/default; any secondary index on `(tenant_id, scan_type, status)`.
- `discovered_assets` column nullability for hostname-only rows (manual target with unresolved DNS).
- SQL for `UpsertDiscoveredAsset` including returning old vs new attributes so the handler can derive events without a second query. Alternative: event derivation lives in SQL; we need a decision. Recommend: return old-row JSON from upsert, derive events in Go for testability.
- `asset_events` primary key + partition strategy for the 3-year trim.
- `asset_gone` reaper ownership (this plan defers; data-model plan should confirm).

**From agent-runtime planner:**
- Confirm `scan_type` field placement on DirectivePayload vs. sniffing from bundle `framework`.
- Confirm `asset_discovered` batch semantics + default `batch_size`.
- Confirm that `scan_error` payload unchanged — `ScanErrorPayload{ScanID,Error}` covers partial-failure case.
- Confirm D11 allowlist fully agent-side; agent surfaces rejected-directive telemetry via `scan_error` with a sentinel code.

**From frontend planner:**
- Filter chip set matches §6 canned params; confirm chip → query-string mapping.
- Pagination envelope shape (`items/page/page_size/total`) is acceptable.
- Asset detail page accepts `events` inline in `GET /api/v1/assets/{id}` or calls a sub-endpoint — recommend inline with a default cap.
