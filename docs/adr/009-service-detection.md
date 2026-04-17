# ADR 009: Service detection via nuclei-network stage

**Status:** Proposed
**Date:** 2026-04-17
**Related:** [ADR 003](./003-recon-pipeline.md) (recon pipeline — naabu/httpx/nuclei),
[ADR 006](./006-asset-first-data-model.md) (asset_endpoints.service field),
[ADR 008](./008-agent-log-streaming.md) (agent log streaming — new stage emits
logs on the same bus).

---

## Context

The discovery pipeline today runs naabu → httpx → nuclei-HTTP. This leaves
a blind spot: non-HTTP ports get a port number from naabu but no service
identification. MSSQL on 1433, PostgreSQL on 5432, SSH on 22, Redis on
6379 — all show up in the Assets UI with `service: -` and `version: -`.

This matters for two reasons:

1. **Operators can't build service-based collections.** "All postgres
   endpoints" requires knowing which endpoints run postgres. Today the only
   signal is the port number, which isn't reliable (postgres on 5433,
   non-standard ports, multiple services behind a proxy).

2. **Compliance scans can't auto-match bundles.** The progression
   (discover → collect → credential → comply) breaks at "collect" because
   the service field that the collection predicate filters on is empty.

httpx only probes HTTP/HTTPS and produces nothing for non-HTTP ports.
nuclei's HTTP templates similarly only work against URLs. The gap is
between naabu's raw port list and the fingerprinting stages.

## Problem

Add a service-detection stage that:

- Identifies the protocol/service running on every open port naabu found,
  not just HTTP ones.
- Populates `asset_endpoints.service` and `asset_endpoints.version` so
  collection predicates (`service = 'mssql'`) work.
- Runs inside the existing agent binary with no new external dependencies.
- Doesn't significantly slow down the discovery pipeline.
- Produces structured output that integrates with the existing
  `asset_discovered` WSS ingest path.

## Decision

### D1. Add a `nuclei-network` stage between naabu and httpx

The agent already downloads and runs nuclei. nuclei supports a `network`
protocol in its template language that sends raw TCP/UDP probes and matches
responses. The nuclei-templates repository ships hundreds of
`network/detection/` templates covering common services.

Pipeline becomes:

```
naabu (port scan)
  → nuclei-network (service detection on ALL ports)
    → httpx (HTTP-specific enrichment on ports identified as HTTP)
      → nuclei-http (vulnerability scan on HTTP URLs)
```

nuclei-network runs against `host:port` pairs (not URLs). It uses
templates tagged `network` + `detection` (or a curated subset we pin).

### D2. Template selection

nuclei-network runs with:

```
-t network/detection/ -severity info,low,medium,high,critical
```

The `network/detection/` directory in the nuclei-templates repo contains
service-identification templates. Each produces a finding with the service
name in the `template-id` (e.g., `mssql-detect`, `postgres-detect`,
`ssh-detect`, `redis-detect`, `mongodb-detect`).

We pin the templates directory the same way we pin the HTTP templates today
(via `EnsureTemplates()` in `agent/internal/runner/recon/install.go`). No
separate template set — the same downloaded templates bundle covers both
network and HTTP passes.

### D3. Output → backfill `asset_endpoints.service` + `version`

nuclei-network findings carry:

```json
{
  "template-id": "mssql-detect",
  "matched-at": "192.168.0.199:1433",
  "info": {
    "severity": "info",
    "tags": ["network", "detection", "mssql"]
  },
  "extracted-results": ["Microsoft SQL Server 2022"]
}
```

The agent emits these as `asset_discovered` batches (stage = `nuclei-network`).
The server's ingest handler:

1. Writes a `finding` row with `source_kind = 'network_vuln'`,
   `source = 'nuclei-network'`.
2. **Backfills** `asset_endpoints.service` and `asset_endpoints.version`
   from the template metadata:
   - `service`: derived from template tags or a static map
     (`mssql-detect → mssql`, `postgres-detect → postgresql`,
     `ssh-detect → ssh`, etc.).
   - `version`: from `extracted-results[0]` if present.

The backfill only writes if the current `service` is NULL — httpx's
fingerprint (which runs later) takes precedence for HTTP ports because
it's more specific.

### D4. httpx input changes

Today httpx receives all naabu findings as input. After D1, httpx should
only receive ports that nuclei-network identified as HTTP (or ports where
nuclei-network produced no match — assume HTTP as a fallback, same as
today).

This makes httpx faster (fewer ports to probe) and more accurate (doesn't
waste time on ports known to be non-HTTP).

Implementation: the agent's pipeline orchestrator filters httpx input
based on nuclei-network results. Ports tagged with non-HTTP services
(mssql, postgres, ssh, etc.) are excluded. Ports with HTTP-related tags
or no match are passed to httpx.

### D5. Agent-side implementation

In `agent/internal/runner/recon/pipeline.go` (or equivalent), the
pipeline stages become:

```go
func (r *ReconRunner) Run(ctx context.Context, target string, pps int) error {
    // Stage 1: port scan
    ports := runNaabu(ctx, target, pps)

    // Stage 2: service detection (NEW)
    serviceResults := runNucleiNetwork(ctx, ports)
    r.streamServiceResults(ctx, serviceResults)  // asset_discovered batch with stage=nuclei-network

    // Stage 3: HTTP fingerprinting (filtered input)
    httpPorts := filterHTTPPorts(ports, serviceResults)
    urls := runHTTPX(ctx, httpPorts)
    r.streamHTTPXResults(ctx, urls)

    // Stage 4: HTTP vulnerability scan
    runNucleiHTTP(ctx, urls)
    // ... existing flow
}
```

### D6. New function: `runNucleiNetwork`

```go
func runNucleiNetwork(ctx context.Context, targets []NaabuFinding, onHit func(NucleiHit)) error {
    bin, _ := EnsureTool("nuclei")
    templatesDir, _ := EnsureTemplates()
    args := []string{
        "-t", filepath.Join(templatesDir, "network", "detection"),
        "-jsonl", "-silent", "-no-color", "-disable-update-check",
    }
    // Feed host:port pairs via stdin (same pattern as nuclei-HTTP)
    // ...
}
```

Uses the same `EnsureTool` / `EnsureTemplates` infrastructure. No new
binary download. The `-t network/detection/` flag selects only service
detection templates.

### D7. Service-name mapping

A static map in the agent or server translates template IDs to
canonical service names:

```go
var templateToService = map[string]string{
    "mssql-detect":      "mssql",
    "postgres-detect":   "postgresql",
    "mysql-detect":      "mysql",
    "mongodb-detect":    "mongodb",
    "redis-detect":      "redis",
    "ssh-detect":        "ssh",
    "ftp-detect":        "ftp",
    "smtp-detect":       "smtp",
    "dns-detect":        "dns",
    "rdp-detect":        "rdp",
    "memcached-detect":  "memcached",
    "elasticsearch-detect": "elasticsearch",
    "rabbitmq-detect":   "rabbitmq",
    "vnc-detect":        "vnc",
}
```

Unmapped template IDs fall back to the template-id itself with the
`-detect` suffix stripped. This handles new community templates without
code changes.

### D8. Performance budget

nuclei-network service detection is lighter than nuclei-HTTP vulnerability
scanning:

- Templates are small (TCP connect + read banner + regex match).
- No TLS negotiation in most cases.
- Runs against `host:port` not URLs (no HTTP overhead).

Expected: ~2-5 seconds per host for top-100 detection templates.
Acceptable within the existing pipeline time budget (naabu ~30s for a /24,
httpx ~10s, nuclei-HTTP ~60s). The new stage adds ~5% to total pipeline
time.

If the template set grows large, we can gate it with a curated allowlist
(`-t network/detection/mssql-detect.yaml,postgres-detect.yaml,...`)
instead of the full directory.

### D9. Allowlist interaction

The scan allowlist (`/etc/silkstrand/scan-allowlist.yaml`) gates ALL
pipeline stages. nuclei-network only probes ports that naabu found AND
that fall within the allowlisted CIDRs. No change to the allowlist
enforcement — it's checked once at pipeline start, same as today.

## Consequences

**Positive:**

- Every open port gets a service label, not just HTTP ports.
- Collection predicates like `service = 'mssql'` work reliably.
- Bundle-to-endpoint auto-matching becomes possible (CIS-MSSQL targets
  endpoints where `service = 'mssql'`).
- No new binary — reuses existing nuclei + templates infrastructure.
- Community-maintained templates cover new services without code changes.

**Negative:**

- Two nuclei passes per scan (network + HTTP). Total pipeline time goes
  up ~5%. Acceptable at current scale; can parallelize later.
- Template-to-service mapping is a static map that needs maintenance.
  Mitigated by the fallback (strip `-detect` suffix).
- nuclei-network may produce false positives on unusual services.
  The backfill only writes if `service IS NULL`, so httpx takes
  precedence for HTTP ports (more reliable).

**Scope boundary:**

- No new binary/tool download. nuclei is already installed.
- No changes to the allowlist format or enforcement.
- No deep vulnerability scanning for non-HTTP services in this ADR.
  nuclei-network runs detection templates only. Protocol-specific
  vulnerability templates (e.g., "MSSQL weak auth") are a follow-on.
- No UDP service detection. naabu defaults to TCP; UDP scanning is
  a separate future decision.

## Implementation (PR split)

1. **PR 1 — agent pipeline**: add `runNucleiNetwork` stage, wire into
   pipeline between naabu and httpx, filter httpx input.
2. **PR 2 — server ingest**: parse `stage=nuclei-network` batches,
   backfill `asset_endpoints.service` + `version`, write findings.
3. **PR 3 — template map + tests**: static service-name map, unit tests
   for mapping + fallback, integration test with a known port.

## Open questions

- **OQ1.** Should the nuclei-network stage run in parallel with httpx
  (to save time) or strictly before it (to filter httpx input)? Lean
  strictly-before for correctness; parallel is an optimization for later.
- **OQ2.** Do we curate a subset of network/detection templates or run
  the full directory? Full directory is simpler but may include noisy
  templates. Lean full directory + a denylist for known-noisy ones.
- **OQ3.** Should the service-name map live in the agent (maps
  template-id before emitting) or in the server (maps on ingest)?
  Lean agent-side — keeps the server's ingest path generic and the
  agent already knows which templates it ran.
