# Scan progress + SSE event bus

Three-PR effort to give scans visible progress (both in agent logs and in
the UI), on top of a reusable SSE framework we can later point at rule
fires, notifications, asset discovery, and ADR 005 audit events.

## Motivating gap

Today:
- A discovery scan shows `RUNNING` until it terminates; the UI has no
  signal about which pipeline stage is active or how far along.
- Agent stdout is not structured per-stage; troubleshooting needs
  `journalctl` grep-fu.
- The only way to know a scan is stuck is to `ps aux` on the agent host.

After this effort: structured `scan_progress` events flow agent → server
→ SSE → UI, with per-stage state + counts. Same events land in agent
logs as `slog.Info("scan.progress", …)` lines for terminal diagnosis.

## Decisions

### D1. Three PRs, landing independently

1. **PR A — SSE framework**: server-side event bus + `/api/v1/events/stream`
   endpoint + short-lived stream-token auth. No agent or UI changes.
2. **PR B — scan_progress emission**: agent WSS message + server
   persistence + republish via the bus. No UI changes.
3. **PR C — UI progress view**: `useEvents` React hook + ScanResults
   per-stage checklist.

### D2. Event envelope

```jsonc
{
  "kind": "scan_progress",         // dotted namespace; freely extensible
  "resource_type": "scan",          // "scan" | "asset" | "rule" | "channel" | …
  "resource_id": "<uuid>",
  "occurred_at": "<rfc3339>",
  "payload": {                       // kind-specific, versioned via fields
    "stage": "naabu",
    "state": "completed",
    "count": 42,
    "message": "…"
  }
}
```

Matches the ADR 005 audit-event shape so we can eventually pipe both
through the same bus. Open: whether `audit.*` events also surface over
SSE once ADR 005 lands — probably yes, behind role check.

### D3. Persistence

`scan_progress_events` table, monthly-partitioned:

```sql
CREATE TABLE scan_progress_events (
    tenant_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    seq INT NOT NULL,               -- monotonically increasing per scan_id
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stage TEXT NOT NULL,            -- naabu | httpx | nuclei | fetch_bundle | execute | …
    state TEXT NOT NULL,            -- started | completed | failed
    count INT,                      -- nullable; present for completion events
    message TEXT,                   -- nullable; human-readable
    PRIMARY KEY (tenant_id, scan_id, seq)
) PARTITION BY RANGE (occurred_at);
```

Write path: agent WSS handler persists, then `events.Bus` republishes.

**Retention: 30 days.** Progress is diagnostic, not compliance-grade.
Nightly partition-drop worker (same pattern we'll add for ADR 005).

### D4. Pipeline stages

**Discovery** (ADR 003 R1a):
- `install_tools` — first-run per-agent install of naabu/httpx/nuclei.
- `naabu` — port + host sweep.
- `httpx` — service/version fingerprinting on open HTTP(S) ports.
- `nuclei` — CVE + tech-detection template run.

**Compliance**:
- `fetch_bundle` — cache hit / miss + download.
- `execute` — Python runner against the target.

Each stage emits:
- `{stage, state: "started"}` at entry.
- `{stage, state: "completed", count: N}` at exit (count = findings /
  controls evaluated).
- `{stage, state: "failed", message: "..."}` on error (pairs with the
  existing `scan_error` terminal message).

### D5. Bus: Upstash Redis pub/sub

Reuse the existing Redis pub/sub (already used for scan directives +
probe results). Tenant-scoped channel `tenant:<uuid>:events`. Tiny new
wrapper `internal/events/bus.go` with `Publish(ctx, tenantID, env) error`
and `Subscribe(ctx, tenantID) (<-chan Envelope, func())`.

Why not Postgres `LISTEN/NOTIFY`? Cloud SQL via the connector does
support it, but it holds a connection open per subscriber and Upstash is
our existing choice for multi-pod fan-out. One system, one pattern.

### D6. SSE endpoint + auth

- `POST /api/v1/events/stream-token` → returns `{ token, expires_at }`
  signed with the same JWT secret. 5-minute TTL. Stateless — no DB.
- `GET /api/v1/events/stream?kinds=scan_progress&token=<t>` → SSE frames.
  Kinds param is comma-separated; omit to subscribe to all.
- Auth: validate the stream token, extract tenant_id, subscribe to that
  tenant's Redis channel, filter by kind, write frames until client
  disconnect or server shutdown.
- Heartbeat: SSE `: ping\n\n` comment every 15s to keep proxies happy.

**Why token in query string and not header?** `EventSource` can't set
custom headers. Cookies would require reworking our JWT-in-header auth
storage. Short-lived query-string tokens are auditable and don't expand
the auth surface.

### D7. Client hook

`useEvents({ kinds?, resourceType?, resourceId? }) → { events, connected, error }`

- Fetches a stream token.
- Opens an `EventSource` with `?token=…&kinds=…`.
- Filters incoming events by `resourceType` / `resourceId` client-side
  (server already filters by kind; resource-level filter lives on
  client since the server channel is tenant-scoped).
- Auto-reconnect on error via EventSource's built-in retry, with a
  fresh token on each reconnect.

### D8. Rate limiting

No limit in v1. If we see misbehaving tabs holding many connections
open, add a per-user cap. Streams are cheap enough at our current
tenant count that this is premature.

### D9. Agent logging

Every `scan_progress` WSS emit also logs `slog.Info("scan.progress",
"scan_id", …, "stage", …, "state", …, "count", …)` on the agent side
so `journalctl -u silkstrand-agent | grep scan.progress` gives the same
timeline without needing the UI.

## Migration order (deploy-safe)

1. PR A lands: table, bus, SSE endpoint, stream-token endpoint. Nothing
   reads from the table yet; no publishers.
2. PR B lands: agent emits, server persists and publishes. The SSE
   endpoint now produces events but the UI doesn't subscribe.
3. PR C lands: UI subscribes and renders.

Each PR verifiable end-to-end against stage without requiring the next.

## Out of scope (for this plan)

- **Raw log streaming** from the agent. Separate design effort — needs
  its own ADR because buffering, redaction, and delivery semantics are
  real tradeoffs.
- **Cross-tenant super-admin event stream** for backoffice. Plausible
  future add; needs role-gated endpoint.
- **Persistence of non-scan events** (rule fires, notifications) — those
  land in ADR 005 audit events. The SSE framework built here can later
  republish them so the same hook works; schema differences stay
  behind the `kind` discriminator.

## Second event kind: `agent_log` (ADR 008)

The same SSE framework is the transport for agent log streaming per ADR
008. Both event kinds share the bus; consumers filter by `kind`. Concrete
deltas when `agent_log` lands:

- **Agent handler** — `slog.NewMultiHandler(stdout, tunnel)` installs a
  tunnel handler that emits `agent_log` events at info+ only. Debug stays
  local.
- **Server** — WSS parses `{type: "agent_log"}`, stamps tenant/agent ids,
  republishes. No DB write by default; `agent_log_retention_days=0|7|30`
  is a future opt-in per-tenant knob.
- **UI** — two consumers:
  - **Agents page → Console tab** subscribes to
    `/events/stream?resource_type=agent&resource_id=<id>`.
  - **Scan Results → Console tab** subscribes to
    `/events/stream?kind=agent_log&scan_id=<id>` (agent stamps `scan_id`
    on the payload when emitting inside a scan-scoped goroutine).
- **Rate limit** — 50 lines/sec/agent token bucket on the agent side;
  drops surface as a summary `agent_log.throttled` event every 5s.

PR split for agent-log work layers on top of this plan's PRs:

- After PR A (framework): the bus accepts `agent_log` events natively
  via the same envelope — no framework changes.
- **PR D** — agent-side tunnel handler + info+ filter + rate limit.
- **PR E** — UI Console tabs on Agents page + Scan Results.

Total: 5 PRs across both features, with PR A doing the heavy lifting
once.

## Open questions (revisit during PR review)

1. Do we compact progress events — e.g. drop `stage=naabu,state=started`
   rows once the matching `completed` arrives? My lean: no, keep both;
   latency between them is the useful diagnostic.
2. `seq` generation — agent-side counter on the emit or server-side
   `MAX(seq)+1` on insert? Server-side is simpler but adds a round-trip.
   Agent-side is trivial if the agent owns the monotonic counter per
   scan. Lean: agent-side.
3. Should completed scans' progress be rendered by default, or only
   when a user expands "details"? Lean: hidden by default once status
   is terminal; collapsed summary ("4 stages ✓ in 42s") inline.
