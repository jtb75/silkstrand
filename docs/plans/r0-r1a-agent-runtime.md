# R0/R1a Agent Runtime + ProjectDiscovery Tools Distribution

## 1. Summary

R1a ships the agent-side recon runtime:

- A new `agent/internal/runner/recon.go` alongside `python.go`, selected via bundle `manifest.yaml` `framework: recon-pipeline`.
- A dispatcher change in `agent/cmd/silkstrand-agent/main.go` that picks runner by framework.
- A three-stage L3 pipeline (naabu → httpx → nuclei) invoked as subprocesses.
- PD tool binaries distributed via **agent-managed runtime install** into `/var/lib/silkstrand/runtimes/<tool>/<version>/` (option 2), ratifying the ADR lean.
- Nuclei templates distributed as **SilkStrand-hosted signed bundles** (compliance-bundle-pattern), refreshed daily.
- Streaming `asset_discovered` batches over the existing WSS tunnel; terminal `discovery_completed` or `scan_error`.
- Local allowlist (`/etc/silkstrand/scan-allowlist.yaml`) gates every target before any tool is invoked; global pps cap compiled into the binary.

## 2. PD Tools Distribution Decision

**Chosen: Option 2 — runtime download, agent-managed, versioned, hash-pinned.**

Layout:

```
/var/lib/silkstrand/runtimes/
  naabu/2.3.x/naabu
  httpx/1.6.x/httpx
  nuclei/3.3.x/nuclei
  .manifest.json          # {tool, version, sha256, installed_at}
```

Bootstrap sequence on first recon directive:

1. Recon runner consults `.manifest.json`; if tool missing or sha mismatch, fetch from `storage.googleapis.com/silkstrand-runtimes/<tool>/<version>/<os>-<arch>/<tool>(.exe)`.
2. SHA-256 verified against a pin table compiled into the agent at build time (`agent/internal/runner/recon/pdpins.go`). Pins bumped on agent release.
3. `chmod 0755`, atomic rename.
4. Per-tool mutex prevents concurrent fetch.

Reuses `agent/internal/cache/cache.go` primitives (GCS HTTP GET + sha check) — factor a small `pkgfetch` helper shared by bundles and runtimes.

### Tradeoff table

| Option | Agent binary size | Cold-start latency | Update cadence | Offline install | Build complexity | Verdict |
|---|---|---|---|---|---|---|
| 1. `go:embed` | +~300 MB (3 tools × 6 platforms × ~15 MB) or per-platform +~50 MB | 0 | Tied to agent release | Works | Moderate (embed per platform) | Reject |
| **2. Runtime download** | **~0** | **~5-15s first scan** | **Independent, fast** | **Needs HTTPS to GCS at first scan** | **Low** | **Chosen** |
| 3. Go SDK link | +~120-200 MB (nuclei pulls >400 deps) | 0 | Tied to agent release | Works | High — nuclei deps churn, CGO edges | Reject |
| 4. System install | 0 | 0 | Customer problem | N/A | Low | Reject (fragile) |
| 5. Sidecar container | — | — | — | — | — | Reject (agent is single-binary) |

### Risks / mitigations

- **First-scan failure in airgapped sites** → document `SILKSTRAND_RUNTIMES_DIR` override; support operator manually staging binaries; fall back to filesystem presence check.
- **GCS availability SLO** → same dependency we already have for bundle distribution; acceptable.
- **Supply chain** → sha pins live in agent binary (code-signed), not in a network-fetched manifest. A compromised GCS cannot serve unknown binaries.
- **Disk usage** → cap at 2 versions per tool; GC older versions on successful run.

### Sizing (approx)

| Tool | Per-platform binary |
|---|---|
| naabu | ~17 MB |
| httpx | ~25 MB |
| nuclei | ~45 MB |
| Templates (nuclei) | ~60 MB unpacked (~10 MB gz) |

Agent stays ~20 MB. Runtime dir ~90 MB per host.

## 3. Nuclei Template Distribution Decision

**Chosen: SilkStrand-hosted signed template bundle, daily background pull.**

- Location: `/var/lib/silkstrand/runtimes/nuclei-templates/<semver>/`.
- Source: `storage.googleapis.com/silkstrand-runtimes/nuclei-templates/<semver>.tar.gz` + `.sig` (Ed25519) — reuses the bundle signing pipeline from `agent/internal/cache/cache.go`.
- SilkStrand CI mirrors and curates `github.com/projectdiscovery/nuclei-templates`, drops `fuzzing/`, `headless/`, and anything needing auth-keys we don't ship; daily build.
- Agent background ticker (1/day, jittered) refreshes. Recon run blocks only if no templates at all.
- `--templates-directory` passed to nuclei; `-disable-update-check -ut=false` to prevent it phoning home.

Rationale over alternatives: github pulls break on corporate firewalls; bundled-with-binary goes stale within a week; daily SilkStrand mirror gives us curation + signing, reuses infra, stays inside the single egress destination customers already allowlist.

## 4. Recon Runner Architecture

### Package layout

```
agent/internal/runner/
  runner.go                  (existing — extend interface slightly)
  manifest.go                (existing — already has Framework field)
  python.go                  (existing)
  recon.go                   (new — thin adapter implementing Runner)
  recon/
    pipeline.go              orchestrates naabu → httpx → nuclei
    naabu.go                 subprocess wrapper + JSONL stream parser
    httpx.go                 subprocess wrapper + JSONL stream parser
    nuclei.go                subprocess wrapper + JSONL stream parser
    stream.go                batcher; publishes asset_discovered via callback
    redact.go                evidence redaction (ADR open item 4.3)
    pdpins.go                compile-time sha256 pin table per (tool, version, platform)
    install.go               runtime install/verify into /var/lib/silkstrand/runtimes
    allowlist.go             loader + matcher for scan-allowlist.yaml
    ratelimit.go             global pps cap (D11 defense-in-depth)
    schema.go                silkstrand-discovery-v1 output types
```

### Runner interface change

`Runner.Run` currently returns `(json.RawMessage, error)`. Recon needs to *stream*. Minimal change: add a second method or a constructor-injected emit callback:

```go
type EmitFunc func(msgType string, payload any) error

type Runner interface {
    Run(ctx context.Context, req RunRequest) (json.RawMessage, error)
}

// recon runner is constructed with an EmitFunc for interim messages;
// returns final silkstrand-discovery-v1 summary as the Run() result,
// which main.go publishes as scan_results for audit parity.
```

### Dispatcher wiring (`main.go`)

Replace the single `pythonRunner` with a framework→runner map. `handleDirective` loads `manifest.yaml` via `runner.LoadManifest`, reads `Manifest.Framework`, and selects. Recon runner is constructed with a closure over `tun.Send` that emits `asset_discovered` / `discovery_completed`.

### Pipeline flow

1. Parse directive; resolve `TargetConfig` shape (CIDR / range / IP / hostname per D5).
2. **Allowlist check** (§6). Reject with `scan_error` if any part of target escapes.
3. Ensure PD tools + templates installed (install.go; lazy, cached).
4. Stage 1 — **naabu**: `-host <target> -json -rate <pps_cap>` → stream live ports.
5. Stage 2 — **httpx** fed from naabu JSONL (`-json -tech-detect -tls-grab`).
6. Stage 3 — **nuclei** against httpx-discovered URLs (`-jsonl -severity medium,high,critical -etags intrusive`) using `-templates-directory` from the signed templates dir. Read-only templates; no fuzz.
7. `stream.go` consumes each stage's JSONL, normalizes into `silkstrand-discovery-v1` asset dicts, batches, emits.
8. On stage-level error: log, emit `scan_error`, keep partial findings already streamed (D9 semantics).

## 5. WSS Message Contract

### `asset_discovered` payload

```json
{
  "type": "asset_discovered",
  "payload": {
    "scan_id": "uuid",
    "batch_seq": 3,
    "assets": [
      {
        "ip": "10.0.0.5",
        "port": 443,
        "hostname": "internal-app.corp.local",
        "service": "https",
        "version": "nginx/1.25.3",
        "technologies": ["nginx", "cloudflare"],
        "cves": [{"id": "CVE-2024-XXXX", "severity": "high", "template": "cves/..."}],
        "evidence": { "...redacted..." },
        "observed_at": "2026-04-13T12:00:00Z"
      }
    ]
  }
}
```

### Batching strategy

- **Flush on `max(N=10 assets, T=2s)`** — whichever hits first.
- After nuclei stage (CVE results arrive late and large), flush each enriched asset individually (batch size 1) so UI shows CVEs promptly.
- `batch_seq` is a monotonic counter per `scan_id` so the API can detect gaps.
- Bound in-flight bytes: block on tunnel send; do not buffer more than ~4 MB.

### Terminal messages

- `discovery_completed` — `{scan_id, total_assets, stages:{naabu_ms, httpx_ms, nuclei_ms}, template_version}`.
- `scan_error` — existing shape; partial assets already delivered are retained by API per D9.

Final `scan_results` (with `silkstrand-discovery-v1` summary) still goes out so existing audit trail works unchanged.

## 6. Allowlist Enforcement

### Gate location

**Before `naabu` is spawned, once per directive.** Targets enumerated; each expanded into concrete CIDRs; rejected unless every address is covered by `allow` and none by `deny`. Hostnames are resolved first (`net.DefaultResolver.LookupIP`), then each resolved IP is checked; the hostname itself may also match a literal `allow` entry.

We do **not** do per-packet libpcap filtering — naabu writes raw SYN, and interposing a pcap filter is out of scope for R1a. Justification: naabu is given only pre-validated inputs.

### Global pps cap

Compile-time constant `const MaxGlobalPPS = 1000` in `recon/ratelimit.go`. The effective rate passed to naabu = `min(allowlist.rate_limit_pps, directive.rate_limit_pps, MaxGlobalPPS)`. Enforced via naabu's `-rate` flag; a second watchdog goroutine monitors naabu's reported rate from JSONL and kills the process if it exceeds 2× the cap (guards against flag-parse bugs).

### YAML schema

```yaml
# /etc/silkstrand/scan-allowlist.yaml
allow:
  - 10.0.0.0/16           # CIDR
  - 192.168.50.5          # single IP
  - 192.168.50.10-192.168.50.50   # range
  - corp-internal.example.com     # exact hostname, case-insensitive
  - "*.internal.example.com"      # wildcard hostname (single label)
deny:
  - 10.0.99.0/24
rate_limit_pps: 1000      # optional; clamped to MaxGlobalPPS
```

Loader lives in `recon/allowlist.go`; cached with mtime-based reload; fail-closed on parse error (directive rejected with explicit error message for admin visibility).

## 7. Cross-Platform Notes

| Tool | linux amd64/arm64 | darwin amd64/arm64 | windows amd64/arm64 |
|---|---|---|---|
| naabu | Full (CAP_NET_RAW or root) | Partial — SYN needs root, connect-scan works unprivileged | Works but requires Npcap; connect-scan fallback |
| httpx | Full | Full | Full |
| nuclei | Full | Full | Full |

**R1a policy:**

- Linux: prefer SYN scan (`-scan-type s`) if process has `CAP_NET_RAW`; else connect scan. Document setcap step in install guide (defer doc to later PR per scope).
- macOS: connect scan by default (no root assumption for dev machines; agents on macOS are rare but supported).
- Windows: connect scan only for R1a. Skip Npcap dependency. Document in the deferred authoring doc.
- arm64 Windows: PD does not publish arm64 windows builds uniformly — treat as unsupported-at-runtime; agent binary still cross-compiles, but recon directives return `scan_error: platform_unsupported`. Compliance scans unaffected.

No Docker required. Single-binary story preserved.

## 8. Concurrency and Resource Limits

- Recon runs inside a dedicated goroutine pool sized 1 per host (long scans serialize; starvation of compliance scans avoided at the cost of throughput — acceptable for v1).
- Context cancellation propagates `SIGTERM` to subprocesses with a 10s `SIGKILL` grace.
- Memory ceiling per scan: buffered JSONL channels bounded at 1024 entries each; back-pressure stops stage upstream.
- A simple semaphore keyed by scan type in `main.go`: `max_concurrent_recon=1`, `max_concurrent_compliance=N` (existing). Compliance directives always preempt enqueue-order for worker selection.

## 9. Failure Modes

| Scenario | Agent action |
|---|---|
| PD tool install fails (network) | `scan_error: runtime_install_failed: <tool>` — no partial results |
| Templates missing entirely | `scan_error: templates_unavailable` |
| naabu crashes mid-scan | emit `scan_error` with partial; already-streamed `asset_discovered` messages persist (D9) |
| httpx/nuclei error on specific asset | JSONL parse-and-continue; per-asset error folded into `events[]` with `event_type: scan_degraded` |
| Target not in allowlist | immediate `scan_error: allowlist_violation: <target>` before any subprocess starts |
| pps watchdog trips | kill naabu, `scan_error: rate_cap_exceeded`, keep partial |
| Timeout (hard cap 24h) | graceful terminate, `discovery_completed` with `truncated: true` |

## 10. Evidence Redaction (ADR open item 4.3)

In `recon/redact.go`, applied to nuclei `matcher-status`/`extracted-results`/response bodies before emission:

- Regex deny-list: AWS AKIA/ASIA, JWT (`eyJ...`), PEM headers, `postgres://user:pw@`, bearer tokens, generic `password=` kv.
- Truncate response bodies to 4 KiB default (`SILKSTRAND_EVIDENCE_BODY_MAX`).
- Unit-tested with a golden fixtures directory under `recon/testdata/redact/`.

Customer-overridable rules file: `/etc/silkstrand/redaction-rules.yaml` (optional); format deferred.

## 11. Open Items / Dependencies

**From API planner:**
- Exact `DirectivePayload` extension for recon: expected new fields `target_type: network_range`, `target_config: {shape: cidr|range|ip|hostname, value: string, rate_limit_pps?: int, max_ports?: int}`.
- Message-type registration for `asset_discovered` and `discovery_completed` on the API WSS receiver; confirm batch-seq / ordering contract and max payload size.
- Confirmation that `scan_results` final summary is still required (for audit parity) or whether `discovery_completed` supersedes it for recon.

**From data-model planner:**
- Final field list on `discovered_assets` the agent should populate vs. the API derives — specifically whether `technologies`/`cves` are raw from PD or normalized agent-side.
- `asset_events` types the agent is expected to originate directly (probably only `new_asset`; deltas are API-side).

**From frontend planner:**
- Live-progress rendering expectations (does UI want incremental port count? stage names?). If yes, agent should include `stage` field in batches.

**Inside agent (not blocking):**
- Whether to extract `pkgfetch` as a shared helper now or inline in `recon/install.go` for R1a and refactor later — recommend inline for solo-dev speed.
