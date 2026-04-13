# R0 + R1a — Cross-team synthesis

Companion doc to the four sibling plans (`r0-r1a-data-model.md`, `r0-r1a-api.md`, `r0-r1a-agent-runtime.md`, `r0-r1a-frontend.md`). Highlights the seams where the four planners touch, the contradictions to resolve, and the order of work.

## What R0 ships

One migration (`011_recon_pipeline.up.sql`) lands the entire schema for ADR 003 phases R0–R1c in a single atomic file:

- `discovered_assets` (current-state inventory, partitioned only via the unique key)
- `asset_events` (append-only, **monthly partitions** from day 1)
- `asset_sets` (saved predicates)
- `correlation_rules` (versioned `match → action` bodies)
- `notification_channels` + `notification_deliveries` (deliveries also partitioned monthly)
- `one_shot_scans` + `scans.parent_one_shot_id`
- `targets.asset_id` + backfill (every existing target gets an asset row, source=`manual`)
- `scans.scan_type` (`compliance` | `discovery`, default `compliance` for back-compat — implied by the API plan; data-model planner should adopt)

Indexes optimized for the Assets-page filter chips. JSONB GIN on `cves` and `technologies` with `jsonb_path_ops`. Webhook secrets stored as base64 AES-GCM ciphertext inside `notification_channels.config` (mirrors `credential_sources`).

## What R1a ships

| Layer | Surface |
|---|---|
| API | New `discovery` scan type on existing pipeline; new WSS messages `asset_discovered` / `discovery_completed`; new ingest handler; `GET /api/v1/assets` and `/{id}`; manual-target creation now writes asset row first |
| Agent | `agent/internal/runner/recon.go`; naabu → httpx → nuclei subprocess pipeline; PD tools auto-installed to `/var/lib/silkstrand/runtimes/`; D11 allowlist enforcement before any packet; evidence redaction |
| Frontend | `/assets` route with List + Topology tabs (`@xyflow/react`); 7 filter chips; asset detail drawer with event timeline; live updates during running scans; manual-target form gets D6 notice |

## Cross-team decisions to resolve before code starts

These are points where two or more planners assumed different things or punted.

### 1. `UpsertDiscoveredAsset` event-derivation seam
- **API plan** wants the upsert to return both old and new row state so events are derived in Go (testable, simple).
- **Data-model plan** designs the schema but doesn't pick a side.
- **Recommendation:** API's option — derive events in Go from `(old, new)` tuples returned by the upsert. Easier to unit-test, easier to evolve event taxonomy without schema migrations.

### 2. `asset_gone` ownership
- **Data-model plan** introduces `missed_scan_count` and the rule (`N=3` consecutive missed scans → emit `asset_gone`).
- **API plan** punts: "leave in place in R1a, defer reaper to R1b."
- **Conflict:** the schema is ready but no code increments `missed_scan_count` in R1a. Either ship the reaper now (cheap if scan-scope tracking is solved) or ship the schema with a TODO and a follow-up issue.
- **Recommendation:** ship the schema in R0; defer the reaper to R1b. Document the latent feature in CLAUDE.md so it isn't forgotten.

### 3. Scan scope bookkeeping
- Both data-model and API planners flag this independently. To know which assets a discovery scan "should have seen," the system needs to record the scan's coverage CIDR.
- **Recommendation:** add a `scans.discovery_scope JSONB` column in R0 (e.g., `["10.0.0.0/24"]`). Cheap, makes R1b reaper trivial.

### 4. `scans.target_id` nullability
- Future one-shot scans (D13) target an asset, not a target row. Data-model planner flagged this; the column today is `NOT NULL`.
- **Recommendation:** in R0, `ALTER COLUMN scans.target_id DROP NOT NULL`. R1a still always sets it (compliance + discovery both flow from a `targets` row); R1c one-shot can leave it null.

### 5. Allowlist API exposure
- **Frontend plan** wants each asset row to carry `allowlist_status` so the badge renders.
- **Agent plan** designs the file format but doesn't address surfacing it server-side.
- **API plan** doesn't address it either.
- **Recommendation:** out of R1a. Frontend renders `unknown` for the badge in R1a; agent reports a snapshot in a heartbeat extension (R1b). Don't gate R1a shipping on this.

### 6. `scan_results` parity for discovery
- **Agent plan** says it will still emit a final `scan_results` for audit parity.
- **API plan** doesn't expect it.
- **Recommendation:** drop the `scan_results` for discovery — the `asset_events` log is already the audit trail. Less code, no semantic ambiguity.

### 7. Filter-param encoding
- **API plan** prefers CSV-style `?service_in=postgresql,mysql` flat params on `GET`.
- **Frontend plan** assumes a single `?filter=` blob.
- **Recommendation:** API's flat params. Cleaner to validate, easier to log, plays nicely with link-sharing.

### 8. Event derivation: `port_opened` / `port_closed`
- **API plan** lists `port_opened` in the event-derivation table but **only** when prior asset rows exist for the same IP on other ports. `port_closed` is "deferred."
- **ADR D4** lists both as first-class event types.
- **Recommendation:** ship `port_opened` per the API plan's logic; defer `port_closed` to R1b alongside the reaper. Frontend timeline component must handle the `port_closed` enum value gracefully (no surprise on future events).

### 9. Live progress fields on `asset_discovered`
- **Frontend plan** asks whether agent should send `stage` (naabu / httpx / nuclei) per batch for the UI's "scanning…" indicator.
- **Agent plan** doesn't include it.
- **Recommendation:** add `stage` to the batch payload — one extra string field, agent already knows it. Cheap UX win.

### 10. PD tool distribution: confirm GCS bucket plumbing
- **Agent plan** uses `storage.googleapis.com/silkstrand-runtimes/...`. That bucket doesn't exist yet.
- **Recommendation:** add to the R0 PR (or pre-R0): a Terraform module change creating `silkstrand-runtimes` in `silkstrand-prod`, public-read. Plus the CI step that uploads pinned binaries on agent release. This is a small infra task to land before R1a's agent code is written.

## Order of work (PR sequence)

| # | Branch | Scope | Risk |
|---|---|---|---|
| 1 | `infra/silkstrand-runtimes-bucket` | TF: GCS bucket + IAM for PD tool distribution; CI upload step (skeleton, no binaries yet) | low — additive |
| 2 | `feat/recon-schema` | Migration 011, store interface stubs returning ErrNotImplemented, model types | low — additive |
| 3 | `feat/recon-api` | Discovery scan type, WSS message types, ingest handler, asset read APIs, target-creation D6 | medium — touches existing scan dispatch |
| 4 | `feat/recon-agent-runtime` | Recon runner, PD install, allowlist, redaction, batching | medium — net-new code, but isolated module |
| 5 | `feat/recon-frontend` | Assets page, topology, drawer, manual-target notice | low — net-new page |
| 6 | `infra/seed-pd-binaries` | Upload pinned naabu/httpx/nuclei + curated templates to the runtimes bucket | low — data only |

PRs 2/3/4/5 can ship over a couple of weeks; agent-side and frontend can develop in parallel after the API contract lands in #3.

## What's deliberately out of scope for R0+R1a

- Rule engine evaluation (`suggest_target`, `notify`, `run_one_shot_scan` actions) — R1b/R1c
- Notification dispatcher — R1c
- One-shot scan fan-out — R1c
- `asset_gone` reaper — R1b
- Cloud-API discovery (`target_type: aws_account`) — R2
- Customer-overridable redaction rules YAML — R2+
- Topology edges that mean anything (traffic, dependency) — out of ADR; visualization plus
- Backoffice surfaces — none planned; recon is tenant-scoped

## Stage-0 questions for the user

Before any of the six PRs above are written, decide:

1. **Bucket name + project**: `silkstrand-runtimes` in `silkstrand-prod`, public-read? Or a per-env mirror?
2. **PD binary build/release cadence**: who tracks upstream PD releases and triggers the SilkStrand-side CI rebuild? Manual on first cut, or scripted from day 1?
3. **Nuclei template curation policy**: drop `fuzzing/`, `headless/`, anything needing API keys we don't ship — confirm or list additional exclusions?
4. **Are we OK adding `@xyflow/react` as a runtime dep** (~90 KB gzipped) or do you want a non-React-Flow fallback path documented first?
