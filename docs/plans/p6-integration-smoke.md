# P6 — Integration smoke plan (asset-first refactor)

End-to-end smoke for the asset-first refactor (ADR 006 + ADR 007 +
`docs/plans/ui-shape.md`). Runs against a local Docker-host lab after P1–P5
have merged. The plan is intentionally linear: a single operator should be
able to work through it in one ~2-hour session without backtracking.

**Inputs assumed:**

- `main` contains migration `017_asset_first.sql` and the full handler
  surface (P1–P5).
- Agent binary on `main` speaks the new `asset_discovered` payload shape
  and emits bundle results that the server writes through to `findings`.
- One reachable lab host in the CIDR `192.168.0.0/24` with at least one
  live postgres (port 5432) that accepts the seeded credential.

**Scope boundaries:**

- F1–F9 run against a local dev stack. F10 (prod tag → readyz) is the
  post-merge gate.
- No UI pixel-diffing here. UI assertions check that the right data lands
  in the right pane / filter chip / bulk bar state. Visual polish is not
  P6's remit.
- No load test. The scheduler is exercised with one definition.

---

## Pre-smoke checklist

Run in order. Every step is idempotent; green means move on.

1. **Repo clean + on tag.** `git status` clean, `git log -1` on the merge
   commit that closes P5.
2. **Dev deps.** `make dev-deps` — Postgres 15432 / 15433, Redis 16379 up.
3. **Seed.** `make seed`. Confirms the dev tenant, discovery bundle
   (`11111111-…`), and the seeded agent row (`…000010`, key
   `test-agent-key`) exist.
4. **Migrations applied.** `psql postgres://postgres:postgres@localhost:15432/silkstrand -c "\dt"` shows the new tables:
   `assets`, `asset_endpoints`, `asset_discovery_sources`, `collections`,
   `scan_definitions`, `findings`, `credential_mappings`, `asset_events`
   (partitioned), `notification_deliveries` (partitioned). **And does
   NOT show** `discovered_assets`, `asset_sets`, `one_shot_scans`,
   `notification_channels`, `scan_results`.
5. **API up.** `make run` in terminal 1. `curl -s localhost:8080/readyz`
   returns `{"status":"ok"}`.
6. **Agent up.** Terminal 2, standard command from CLAUDE.md Quick E2E.
   Agent logs show `connected` and a heartbeat within 30s. API logs show
   `agent connected` with the seeded agent id.
7. **Frontend up.** `cd web && npm run dev` — UI at localhost:5173.
8. **JWT.** `TOKEN=$(make jwt)` in terminal 3; `export TOKEN`.
9. **Allowlist.** `/etc/silkstrand/scan-allowlist.yaml` on the host where
   the agent runs contains `192.168.0.0/24`. Confirm via
   `curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/agents/<agent_id>/allowlist`.
10. **Lab target reachable.** `nc -vz 192.168.0.X 5432` succeeds from the
    agent host.

If any step fails → stop. Fix before proceeding; the flows below assume
all ten pass.

### Useful shell aliases for the session

```bash
PSQL="psql -h localhost -p 15432 -U postgres -d silkstrand -AXqt"
API="curl -sS -H 'Authorization: Bearer '$TOKEN -H 'Content-Type: application/json' localhost:8080"
```

---

## Flow index

| # | Flow | Touches |
|---|---|---|
| F1 | Bootstrap (fresh tenant + agent) | backoffice, agents, UI empty states |
| F2 | Discovery scan end-to-end | recon pipeline, assets, asset_endpoints, findings (nuclei) |
| F3 | Scheduler tick | scan_definitions, scheduler goroutine |
| F4 | Compliance scan end-to-end | bundle runtime, findings (bundle_compliance) |
| F5 | Rule fires on new asset | correlation_rules, collections, notification_deliveries |
| F6 | Collections + save-from-filter | collections CRUD, Assets page filter persistence |
| F7 | Bulk credential mapping | credential_mappings/bulk, Coverage column |
| F8 | Dashboard Suggested Actions | dashboard handler, deep-link filter state |
| F9 | Findings suppress / reopen | findings status transitions |
| F10 | Prod upgrade loop | CI/CD, `/readyz` on prod URL |

---

## F1. Bootstrap — fresh tenant + agent, empty UI

### Goal
A tenant provisioned via backoffice with zero prior state lands on a
Dashboard / Assets / Collections / Findings stack that all render empty
and do not error.

### Setup
Use the seeded dev tenant OR create a fresh one through backoffice:

```bash
# Backoffice admin login
ADMIN=$(curl -sS localhost:8081/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@silkstrand.io","password":"<from seed>"}' \
  | jq -r .token)

# Create tenant in the "local" DC (already registered by seed)
curl -sS -X POST localhost:8081/api/v1/tenants \
  -H "Authorization: Bearer $ADMIN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"smoke-tenant","data_center_id":"<dc uuid>"}'
```

### Steps
1. Generate an install token via the tenant API:
   `curl -sS -X POST $API/api/v1/agents/install-tokens` → `{token}`.
2. Launch the agent with the token (see agent README snippet); agent
   registers itself via `/api/v1/agents` with the bootstrap flow.
3. Wait for heartbeat.
4. Open UI → log in → confirm four pages render.

### Expected state
- `SELECT count(*) FROM tenants WHERE id='…';` → 1.
- `SELECT count(*) FROM agents WHERE tenant_id='…';` → 1, `status='online'`.
- `SELECT count(*) FROM assets WHERE tenant_id='…';` → 0.
- `SELECT count(*) FROM asset_endpoints WHERE tenant_id='…';` → 0.
- UI: Dashboard KPI cards all read 0; Suggested Actions empty-state copy
  renders ("Nothing to do — run a discovery scan to begin").
- UI: Assets / Endpoints / Findings tabs all render "No results" empty
  state without network errors in devtools.
- UI: Collections page shows "No collections yet" on both tabs.

### Failure modes
- Dashboard 500 → tail API log, likely the dashboard handler choked on a
  NULL aggregate. `curl $API/api/v1/dashboard` reproduces without the UI.
- Install-token bootstrap fails → `SELECT * FROM install_tokens` to see
  if the token consumed; agent log will show the precise 4xx.

---

## F2. Discovery scan end-to-end

### Goal
A CIDR scan populates `assets`, `asset_endpoints`, `asset_discovery_sources`,
emits `new_asset` events, and writes `source_kind='network_vuln'` findings
for every nuclei hit. Scan reaches `status='completed'`.

### Setup
Create a CIDR target (targets are CIDR-only post-P1):

```bash
TARGET=$(curl -sS -X POST $API/api/v1/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"lab-net","target_type":"cidr","identifier":"192.168.0.0/24"}' \
  | jq -r .id)
```

### Steps
1. Create a scan definition (manual schedule):
   ```bash
   DEF=$(curl -sS -X POST $API/api/v1/scan-definitions \
     -H 'Content-Type: application/json' \
     -d '{"name":"lab discovery","kind":"discovery",
          "scope_kind":"cidr","cidr":"192.168.0.0/24",
          "bundle_id":"11111111-1111-1111-1111-111111111111",
          "schedule":null,"enabled":true}' | jq -r .id)
   ```
2. Execute it:
   `curl -sS -X POST $API/api/v1/scan-definitions/$DEF/execute` →
   returns `{scan_id}`.
3. Watch agent logs. Expect: `naabu → httpx → nuclei` stages. Agent emits
   batched `asset_discovered` frames; completes with `discovery_completed`.
4. Poll `curl $API/api/v1/scans/<scan_id>` until `status='completed'`
   (≤3 min typical).

### Expected state

**DB assertions:**

```sql
-- at least one host surfaced
SELECT count(*) FROM assets WHERE tenant_id='…';                 -- ≥1

-- at least one port surfaced; service tagged
SELECT count(*), count(service) FROM asset_endpoints
 WHERE tenant_id='…';                                            -- both ≥1

-- provenance row per discovered endpoint
SELECT count(*) FROM asset_discovery_sources
 WHERE scan_id='<scan_id>';                                      -- ≥1

-- new_asset events landed on the partition
SELECT count(*) FROM asset_events
 WHERE tenant_id='…' AND event_type='new_asset';                 -- matches endpoint count

-- nuclei findings (if any hits on lab host)
SELECT count(*) FROM findings
 WHERE tenant_id='…' AND source_kind='network_vuln'
   AND scan_id='<scan_id>';                                       -- may be 0 if lab clean

-- scan row terminal
SELECT status, finished_at FROM scans WHERE id='<scan_id>';       -- completed, non-null
```

**API assertions:**

- `GET /api/v1/assets` returns ≥1 row; each row includes
  `endpoint_count`, `finding_count`, `coverage` roll-ups.
- `GET /api/v1/findings?source_kind=network_vuln` returns the nuclei rows
  with `asset_endpoint_id` populated.

**UI assertions:**

- Assets page now shows hosts; tab counter badges update.
- Clicking a host opens the detail drawer with six sections (identity,
  endpoints, findings, coverage, events, discovery sources).
- Dashboard KPI "Total Assets" reflects the new count within 10s.

### Failure modes
- `asset_discovered` frames arrive but no rows persist → check server log
  for `UpsertAssetEndpoint` errors (most likely a NOT NULL column in
  migration 017). Repro with a single crafted frame via `wscat`.
- Nuclei finds hits but `findings` empty → confirm the write-through from
  the nuclei processor calls `UpsertFinding(source_kind='network_vuln')`.
  Grep server log for `finding ingested`.
- `asset_events` partition missing → migration 017 creates partitions for
  `now()` and `now()+1m`; if the smoke straddles month boundary, verify
  the next partition exists.

---

## F3. Scheduler tick

### Goal
The scheduler goroutine ticks on a short cron, materializes a `scans` row
tied to the `scan_definition`, advances `next_run_at`, and updates
`last_run_status`.

### Setup
Disable the definition from F2 to keep the inventory stable:
`curl -X PUT $API/api/v1/scan-definitions/$DEF -d '{"enabled":false}'`.

### Steps
1. Create a tight-cron discovery definition:
   ```bash
   DEF2=$(curl -sS -X POST $API/api/v1/scan-definitions \
     -H 'Content-Type: application/json' \
     -d '{"name":"tick test","kind":"discovery",
          "scope_kind":"cidr","cidr":"192.168.0.0/29",
          "bundle_id":"11111111-1111-1111-1111-111111111111",
          "schedule":"*/2 * * * *","enabled":true}' | jq -r .id)
   ```
2. Note current `next_run_at`:
   `$PSQL -c "SELECT next_run_at FROM scan_definitions WHERE id='$DEF2'"`.
3. Wait up to 3 minutes.
4. Re-query.

### Expected state
- A new `scans` row exists with `scan_definition_id=$DEF2` within 3 min.
- `next_run_at` has advanced past the previous value.
- `last_run_at` is non-null and recent; `last_run_status` ∈
  (`running`, `completed`). Expect `completed` if the short CIDR finishes
  before the next tick.
- Dispatch is idempotent: only ONE scan row per tick, even if the
  scheduler polled multiple times. Verify:
  `SELECT count(*) FROM scans WHERE scan_definition_id='$DEF2'` equals
  the number of ticks that have elapsed (±1).

### Failure modes
- No scan row after 3 min → check `scan_definitions.enabled=true` and
  `schedule` parsed (server log at boot: "scheduler parsed N definitions").
- Duplicate scan rows per tick → the `FOR UPDATE SKIP LOCKED` block is
  broken. Re-read ADR 007 § D4.
- `next_run_at` not advancing → `compute_next(schedule, NOW())` returned
  the same value; check cron parser.

**Cleanup:** disable `$DEF2` when done to keep logs quiet for F4+.

---

## F4. Compliance scan end-to-end

### Goal
A compliance scan against a discovered postgres endpoint writes
`source_kind='bundle_compliance'` findings, and the Scan Results page
Findings tab renders them.

### Setup
1. Pick a discovered postgres endpoint from F2:
   `$PSQL -c "SELECT id,host,port FROM asset_endpoints ae JOIN assets a ON a.id=ae.asset_id WHERE ae.service='postgres' LIMIT 1"`.
   Call the id `$ENDPOINT`.
2. Create a `credential_sources` row (static) with the lab's postgres
   creds and map it to the endpoint:
   ```bash
   CRED=$(curl -sS -X POST $API/api/v1/credential-sources \
     -d '{"type":"static","name":"lab pg","config":{"username":"postgres","password":"postgres"}}' \
     | jq -r .id)
   curl -sS -X POST $API/api/v1/credential-mappings \
     -d "{\"asset_endpoint_id\":\"$ENDPOINT\",\"credential_source_id\":\"$CRED\"}"
   ```

### Steps
1. Create an endpoint-scoped compliance scan definition:
   ```bash
   DEF3=$(curl -sS -X POST $API/api/v1/scan-definitions \
     -d "{\"name\":\"lab pg cis\",\"kind\":\"compliance\",
          \"scope_kind\":\"asset_endpoint\",\"asset_endpoint_id\":\"$ENDPOINT\",
          \"bundle_id\":\"<cis-postgresql-16 uuid>\",
          \"schedule\":null,\"enabled\":true}" | jq -r .id)
   ```
2. Execute: `curl -X POST $API/api/v1/scan-definitions/$DEF3/execute`.
3. Poll the scan until `completed`.

### Expected state
- `findings` rows with `source_kind='bundle_compliance'`, `source='cis-postgresql-16'`,
  `asset_endpoint_id=$ENDPOINT`, mix of `status='open'` (fails) and
  `status='resolved'` (passes) — the pass→resolved mapping per ADR 007 § D2.
- Scan Results page shows the Findings tab populated, with severity chips
  matching the DB severity column.
- Re-running the scan updates `last_seen` and does not duplicate rows —
  upsert key is `(asset_endpoint_id, source_kind, source, source_id)`.

### Failure modes
- Scan errors with credential fetch failure → `credential_mappings` row
  missing or `credential_sources` payload not decryptable (check
  `CREDENTIAL_ENCRYPTION_KEY`). Server emits `credential.fetch` audit
  slog on success.
- Results arrive but `findings` empty → write-through in
  `scan_results` handler not wired to `UpsertFinding`. Grep server log
  for `bundle results ingested`.
- Duplicate findings on re-run → upsert key missing an index, check
  `idx_findings_source` is unique on the four-col key.

---

## F5. Rule fires on new asset → notification delivered

### Goal
On a new postgres endpoint discovery, a correlation rule evaluates, matches
a collection predicate, and enqueues a webhook delivery with HMAC signing.

### Setup
1. Stand up a local webhook catcher (`ngrok http 9999` or
   `https://webhook.site`); note the URL `$HOOK`.
2. Create a webhook credential_source:
   ```bash
   WH=$(curl -sS -X POST $API/api/v1/credential-sources \
     -d "{\"type\":\"webhook\",\"name\":\"hook\",\"config\":{\"url\":\"$HOOK\",\"secret\":\"s3cr3t\"}}" \
     | jq -r .id)
   ```
3. Create an endpoint-scoped collection matching postgres:
   ```bash
   COLL=$(curl -sS -X POST $API/api/v1/collections \
     -d '{"name":"pg endpoints","scope":"endpoint",
          "predicate":{"field":"service","op":"eq","value":"postgres"}}' \
     | jq -r .id)
   ```
4. Create a correlation rule:
   ```bash
   curl -sS -X POST $API/api/v1/correlation-rules \
     -d "{\"name\":\"notify pg\",\"trigger\":\"new_asset\",
          \"body\":{\"match\":{\"collection_id\":\"$COLL\"},
                    \"actions\":[{\"type\":\"notify\",\"credential_source_id\":\"$WH\"}]}}"
   ```

### Steps
1. Delete the existing lab postgres endpoint to force a `new_asset` on
   re-discovery: `DELETE /api/v1/asset-endpoints/<id>` (or truncate
   `assets` + `asset_endpoints` for this lab host via SQL).
2. Re-execute the F2 discovery definition.
3. Wait for scan completion.

### Expected state
- `SELECT status, credential_source_id FROM notification_deliveries WHERE created_at > now() - interval '5 min'`
  → row with `status` in (`pending`, `sent`) pointing at `$WH`.
- Webhook catcher received a POST. Body is JSON with the asset_endpoint
  payload. Headers include `X-SilkStrand-Signature: sha256=<hex>`; verify
  `HMAC-SHA256("s3cr3t", body) == signature`.
- If a retry worker is running, `status='sent'` within 30s. (Retry worker
  is deferred per roadmap — it's OK to see `pending` with a successful
  webhook delivery; the status advance is what's missing.)

### Failure modes
- Rule doesn't fire → collection predicate didn't match. Test directly:
  `POST /api/v1/collections/$COLL/preview` should list the new endpoint.
- Rule fires but no delivery row → action dispatcher panicked; server log.
- Delivery row `failed` → webhook URL unreachable or secret mismatch.

---

## F6. Collections — save from filter, reload reproduces

### Goal
Save the current Assets-page filter as a collection; reopen it later and
confirm the chip state is re-derived from the stored predicate.

### Steps
1. Open UI → Assets → Endpoints tab.
2. Apply filters: `service=postgres`, then a "No scan in last 7d" chip.
   URL query string updates.
3. Click **Save as Collection**; name it "Unscanned Postgres".
4. Network tab: observe `POST /api/v1/collections` with
   `{scope:"endpoint", predicate:{AND:[...]}}`.
5. Reload the page; go to Collections → My Collections → click the new
   collection.
6. Assets page reopens with the same chip state and the same row
   population.

### Expected state
- `SELECT scope, predicate FROM collections WHERE name='Unscanned Postgres'`
  scope is `endpoint`, predicate JSONB contains both conditions.
- `GET /api/v1/collections/<id>/preview` count matches the on-screen row
  count.
- Chip bar state is fully reconstructed from the predicate — not from
  a URL blob.

### Failure modes
- Chips re-render partial → the predicate-to-UI decoder is missing a
  case. Compare predicate JSONB against the chip serializer.
- Preview count ≠ page count → scope mismatch (asset vs endpoint) or
  evaluator dispatch wrong.

---

## F7. Bulk credential mapping

### Goal
Multi-select 3 endpoints on the Assets page, map a credential, and see
the Coverage column update to ✔ on all three.

### Setup
Ensure at least 3 postgres (or same-service) endpoints. If F2 gave only
one, rerun discovery with a wider CIDR or a lab host with multiple
services.

### Steps
1. UI → Assets → Endpoints tab.
2. Select 3 rows via checkboxes; Bulk Actions bar appears.
3. Click **Map Credentials**; pick the existing `lab pg` credential_source.
4. Confirm → modal closes; toast "3 endpoints mapped".
5. Reload.

### Expected state
- `POST /api/v1/credential-mappings/bulk` request body contains 3 endpoint
  ids + one credential_source_id.
- `SELECT count(*) FROM credential_mappings WHERE credential_source_id=$CRED`
  → ≥3 (or the previous count + 3 if any existed).
- Coverage column on those rows reads ✔ after reload.
- `GET /api/v1/assets` response includes `coverage.credential=true` on the
  parent asset when all its in-scope endpoints are mapped.

### Failure modes
- Only N<3 rows inserted → bulk handler silently dedups; check
  `ON CONFLICT DO NOTHING` semantics.
- Coverage ✔ doesn't appear → roll-up in the asset handler is cached; the
  UI's asset list isn't refetched, or the roll-up query is wrong. Hit
  `GET /api/v1/assets/<id>` directly to isolate.

---

## F8. Dashboard Suggested Actions

### Goal
Dashboard surfaces coverage gaps as actionable CTAs that deep-link to the
correct filtered Assets view.

### Setup
Ensure the lab has at least one endpoint WITHOUT a credential_mapping
(F7 mapped some; pick a different service, e.g. mongodb).

### Steps
1. UI → Dashboard.
2. Inspect KPI row — counts non-zero.
3. In Suggested Actions card, find a row like
   "N endpoints missing credentials" with a **Map Credentials** CTA.
4. Click the CTA.

### Expected state
- `GET /api/v1/dashboard` returns a `suggested_actions` array, each
  element having `{kind, count, cta:{label, href}}` shape.
- The CTA's `href` deep-links to Assets with a chip state that matches
  the suggestion predicate (e.g. `?collection=missing-creds`).
- Landing on Assets shows only the endpoints with that gap. The count in
  the tab badge matches the Suggested Actions count (±1 for races).
- Bulk Action "Map Credentials" is one click away with all relevant
  endpoints preselectable.

### Failure modes
- KPI row all zeros despite data → dashboard aggregate query filters on
  the wrong tenant_id. Check slog trace.
- CTA link returns different count than the card → filter-state decoder
  and suggestion-computer disagree on the predicate. Diff the two.

---

## F9. Findings workflow — suppress → reopen

### Goal
Status transitions persist and the UI reflects them.

### Steps
1. UI → Findings → filter to one specific finding from F2 or F4.
2. Click **Suppress** (row action or detail pane).
3. Reload; it no longer appears under the default "open" filter.
4. Switch filter to `status=suppressed`; it appears.
5. Click **Reopen**.
6. Switch filter back to `status=open`; it reappears.

### Expected state
- `SELECT status FROM findings WHERE id='<id>'` — `suppressed` then
  `open` at the corresponding steps.
- `POST /api/v1/findings/<id>/suppress` and `/reopen` return 200.
- The Findings page badge counts update (open down 1, suppressed up 1,
  and vice versa).

### Failure modes
- Status reverts on next scan ingest → the upsert path is overwriting
  `status` on re-scan; ADR 007 § D2 says re-scan should update
  `last_seen` and re-open ONLY if the finding was `resolved`, NOT if
  suppressed. Check the upsert branching.

---

## F10. Prod upgrade loop (post-merge gate)

### Goal
Sanity on the real prod URL after tagging.

### Steps
1. Merge P6 PR to `main`; auto-deploys to stage. Smoke stage via
   F1–F9 (can be a narrowed pass).
2. Tag `v0.1.49` (or the next tag): `git tag -s v0.1.49 -m "asset-first"`
   + `git push --tags`.
3. CI promotes to prod. Wait for Cloud Run revision to serve.
4. `curl -sS https://api.silkstrand.io/readyz` → 200.
5. Log into prod tenant UI; confirm the four pages render (empty if no
   prod data yet, populated if studio tenant has run discovery).

### Expected state
- Cloud Run revision `silkstrand-api` on prod is the one built from the
  tagged commit (`gcloud run services describe silkstrand-api --region us-central1`).
- `/readyz` reports DB + Redis healthy.
- No error spike in the Cloud Run logs for the first 10 minutes.

### Failure modes
- Migration 017 fails on prod → the UUID-randomness guard (P1) tripped;
  inspect the guarded-row query and clean the offending row before
  retrying.
- Prod UI white-screen → bundled frontend bundle drifted from API shape;
  check the deployed `silkstrand-web` revision matches the same commit.

---

## Failure triage appendix

Pattern-match failures against these recipes before opening a bug.

### Scheduler

- **Tick never fires.**
  `$PSQL -c "SELECT id,enabled,schedule,next_run_at FROM scan_definitions"`.
  `enabled=true` and `schedule` non-null required. If `next_run_at` is
  NULL, the scheduler parser rejected the cron.
- **Double dispatch.**
  `SELECT scan_definition_id, count(*) FROM scans GROUP BY 1 HAVING count(*) > <expected>`.
  Re-read `SELECT FOR UPDATE SKIP LOCKED` block; check goroutine count.
- **Drift across restarts.**
  The scheduler recomputes `next_run_at` on startup — confirm it doesn't
  reset to `NOW()` every boot.

### Findings ingest

- **Nuclei findings missing.**
  Agent log: `nuclei exit status`. Should be 0 or empty-result. If 2+,
  runtime crashed. Server log: grep for `UpsertFinding` — one per nuclei
  hit. If zero, the WSS handler didn't route the frame to the findings
  writer.
- **Bundle findings missing.**
  Server log: `bundle results ingested`. If present but `findings` empty,
  the pass→resolved / fail→open switch in `statusFromCheck` is mapping
  everything to a status the insert ignores.
- **Duplicate rows.**
  `SELECT asset_endpoint_id, source_kind, source, source_id, count(*)
   FROM findings GROUP BY 1,2,3,4 HAVING count(*) > 1`.
  Unique index missing on the upsert key.

### Discovery pipeline

- **`asset_discovered` frames dropped.**
  Server log: `asset_discovered received` vs `asset upserted`. Delta
  > 0 means the ingest handler errored on a row. Log the first error.
- **Provenance missing.**
  `SELECT count(*) FROM asset_discovery_sources WHERE scan_id='…'` vs
  endpoint count. Should match. If zero, the handler writes the endpoint
  but skips the provenance insert.
- **Events on wrong partition.**
  `SELECT tableoid::regclass, count(*) FROM asset_events GROUP BY 1`.
  A `_default` partition with rows means the monthly partition for the
  insert timestamp is missing.

### Rules / notifications

- **Rule never matches.**
  `POST /api/v1/collections/<id>/preview` — does the collection currently
  contain the asset the rule should fire on? If not, the predicate
  is wrong, not the rule engine.
- **Matches but no delivery.**
  Check `notification_deliveries` for a `failed` row with `error`
  populated. If nothing at all, the action dispatcher didn't reach the
  notify handler — grep server log for `rule action dispatched`.
- **Delivery `pending` forever.**
  Retry worker is not implemented yet (roadmap § R1.5 deferred). Expected
  for now; the sync dispatch path should flip to `sent` immediately on
  2xx. If `pending` persists, the sync path isn't wired.

### UI

- **Coverage column stale.**
  Hit the API directly: `$API/api/v1/assets/<id>`. If the API is correct
  but the UI is stale, it's a react-query cache issue — check
  `staleTime` / `invalidateQueries` on the mutation.
- **Suggested Actions count ≠ Assets filter count.**
  The two code paths compute the predicate independently. Diff the
  predicate JSON from `/api/v1/dashboard` against the chip-serialized
  predicate posted to `/api/v1/collections/preview`.
- **White screen on a specific page.**
  Devtools console — almost always a TypeScript type mismatch between
  an added API field and the client type. Check `web/src/api/client.ts`
  for drift against the handler DTO.

### Credentials

- **`credential.fetch` audit event emits but scan fails auth.**
  Decryption returned a payload but the payload shape doesn't match the
  engine's expected config. Check `credential_sources.config` JSONB
  against the engine's prober.
- **Bulk map 200 but row count unchanged.**
  The bulk endpoint is dedup'ing silently. Log the insert count server
  side; the handler should return `{inserted, skipped}`.

---

## Sign-off

P6 is merge-ready when F1 through F9 pass in a single linear session
without any open P0/P1 bug. F10 runs immediately after the merge tag.

Record per-flow status in the PR description as a checklist and attach
relevant DB snapshots (pg_dump of the smoke tenant's rows across the new
tables) as an artifact for audit.
