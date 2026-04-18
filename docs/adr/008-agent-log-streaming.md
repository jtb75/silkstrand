# ADR 008: Agent log streaming

**Status:** Accepted (shipped v0.1.77 — persisted logs + SSE + console UI)
**Date:** 2026-04-16
**Related:** [ADR 005](./005-audit-events.md) (audit events — same envelope
shape), [ADR 007](./007-findings-scheduler.md) (scan_progress events — same
SSE transport), `docs/plans/scan-progress-and-sse.md` (the event-bus plan
this extends).

---

## Context

Agent logs land in `/var/log/silkstrand-agent.log` on the host (launchd) or
`journalctl -u silkstrand-agent` (systemd). To see what the agent is doing,
an operator has to SSH to the host. That works for single-operator setups
but is a dead-end for anything multi-tenant, multi-host, or debugging from
the backoffice.

The scan-progress/SSE plan (`docs/plans/scan-progress-and-sse.md`) already
proposes an event bus: agent → WSS → server persistence → SSE fan-out to UI.
Agent log streaming is the same shape: emit structured events from the
agent, persist optionally, fan out over SSE, render in the UI.

## Problem

Design a way to stream agent logs to the tenant UI that:

- Operates on the existing WSS tunnel (agents don't need a second outbound
  connection).
- Preserves local-file logging as the source of truth (debug lines, crash
  forensics, offline analysis).
- Surfaces in two UI contexts — per-agent (always-on Agents-page console)
  and per-scan (Scan Results page during an active scan).
- Respects tenant isolation (one tenant's logs never reach another's UI).
- Survives a moderate log volume without overwhelming the tunnel or the DB.

## Decisions

### D1. Dual-handler in the agent

The agent keeps its existing `slog` handler writing to stdout (which
systemd / launchd captures to the host log file). That's untouched —
debug lines still go there, crashes still produce local artifacts.

A second handler (`tunnel.Handler`) wraps the WSS tunnel and ships log
records as `agent_log` events. This handler is installed alongside the
existing one via `slog.NewMultiHandler(local, tunnel)`.

### D2. Filter: `info+` only on the tunnel

The tunnel handler drops anything below `slog.LevelInfo`. Debug stays
local. Rationale: debug logs are the highest-volume, lowest-value category
for the remote operator; keep them out of the tunnel and the UI. If we
ever need remote debug streaming (e.g., for a customer support session),
that's a separate "debug mode" toggle — out of scope here.

### D3. Event envelope

Matches ADR 005 / ADR 007 event shape. Kind `agent_log`, resource is the
agent:

```jsonc
{
  "kind": "agent_log",
  "resource_type": "agent",
  "resource_id": "<agent_uuid>",
  "occurred_at": "<rfc3339>",
  "payload": {
    "level": "INFO",                   // INFO | WARN | ERROR
    "msg": "received discovery directive",
    "scan_id": "<uuid>",               // optional; present when context available
    "attrs": { ... }                    // slog attrs as a flat map
  }
}
```

The agent includes `scan_id` in the payload whenever the log is emitted
inside a scan-scoped goroutine. That's what lets the Scan Results
per-scan console filter cleanly without a separate envelope field.

### D4. Dual UI surface: Agents console + Scan Results console

Per the user's requirement (both scopes):

- **Agents page → per-agent "Console" tab.** Always-on live tail of an
  agent's `agent_log` events. Auto-scrolls, pauseable, clears on tab
  switch. No filter beyond the built-in info+.
- **Scan Results page → "Console" tab.** Live tail of `agent_log` events
  where `payload.scan_id = <this scan's id>`. Reads the same SSE stream,
  just filters client-side.

Both views subscribe to `/api/v1/events/stream?resource_type=agent&resource_id=<id>`
(agent view) or `/api/v1/events/stream?kind=agent_log&scan_id=<id>` (scan
view). Server-side filtering per the SSE plan's query parameters.

### D5. Persistence: opt-in, short retention

Default behavior is **pass-through** — events hit the bus and fan out to
SSE subscribers; nothing is persisted. If a UI viewer isn't connected,
the log line is lost for remote replay (still on the agent's local file).

This keeps the DB write path off the hot path. An operator who wants
replay opens the console *before* the interesting event; that's how
browser devtools consoles work.

**Opt-in persistence** (future — not shipped in this ADR): a per-tenant
setting `agent_log_retention_days=0|7|30` that, if non-zero, writes the
event to a monthly-partitioned `agent_log_events` table. Out of scope
for the first implementation.

### D6. Rate limiting at the agent

The tunnel handler applies a token-bucket rate limit (default: 50 lines /
second per agent, burst 100). Above that it drops lines and emits one
summary `agent_log.throttled` event every 5s with the drop count. The
local file still gets every line.

This prevents a chatty or misbehaving bundle from saturating the tunnel
or the UI. 50/s is well above normal operating volume (steady-state is
~1/s; scan-start bursts are ~10/s).

### D7. Transport is the existing WSS tunnel

No new outbound connection. The agent extends the existing WS message
set with a new type:

```jsonc
{ "type": "agent_log", "payload": { ...D3 envelope payload... } }
```

Server's WSS handler parses it, stamps tenant/agent ids, and publishes
to the bus — no DB write unless D5's opt-in persistence is configured.

## Consequences

**Positive:**

- Remote visibility into live agents without SSH or log-forwarding setup.
- One transport (WSS + SSE) across scan progress, agent logs, and future
  event kinds (rule fires, audit).
- Local files remain authoritative for debug + forensics.

**Negative:**

- Tunnel volume goes up. Rate-limit + info+ mitigate; normal operation
  should add <1 KB/s per active agent.
- Lossy by default (D5). An operator who opens the console 10 seconds
  after an interesting event missed it. Acceptable for v1; persistence
  is a follow-up.
- `slog.NewMultiHandler` (or equivalent) needs a custom implementation
  since stdlib `slog` doesn't ship one. Small (~40 lines).

**Scope boundary:**

- No debug-level remote streaming. Local file only.
- No retention / search. The UI console is a live tail.
- No agent-to-agent or cross-tenant visibility. Tenant middleware on the
  SSE endpoint enforces isolation.

## Open questions

- **OQ1.** Do we colorize levels in the UI console? Design-system says
  status tokens pair color with label; follow that pattern (INFO neutral,
  WARN warning-bg, ERROR danger-bg with explicit text).
- **OQ2.** Does the Agents console show logs from all agents simultaneously
  (fleet view) or only the selected agent? Lean per-agent only in v1 to
  match the "Console" tab design; a fleet view is a later enhancement.
- **OQ3.** Rate-limit tuning: 50/s/agent was picked from operating
  observation, not measurement. Revisit after the first week of real use.
