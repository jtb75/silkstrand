# ADR 002: Recon → inventory → compliance pipeline

**Status:** Proposed (parked). Not for MVP.
**Date:** 2026-04-12
**Context origin:** Discussion about whether to integrate Nuclei.

---

## Context

Today SilkStrand is a compliance scanner: the admin creates a target by hand,
assigns a bundle, runs a scan. The agent is already inside the customer's
network and authorized to probe it — but we're only using that access for
compliance bundles against known-in-advance assets.

The customer's first question is always "what do I have?" — not "does the
thing I already know about pass CIS?". Getting to compliance *from*
discovery is a much shorter path than asking admins to pre-build the
inventory.

## Decision (intended, not yet executed)

Evolve SilkStrand from a **compliance scanner** into a **recon + compliance
platform** built around the same edge agent. Add a discovery pipeline using
the ProjectDiscovery open-source toolkit — naabu, httpx, nuclei — and a
correlation layer that promotes discovered assets into compliance targets.

The compliance story doesn't go away; it becomes the validation step at the
end of the pipeline instead of the whole product.

## Tooling

All open-source, all Go, all MIT-licensed (compatible with our Apache-2.0):

| Tool | Role |
|---|---|
| `naabu` | Port scanning — CIDR → open ports |
| `httpx` | HTTP probe, tech fingerprinting, version extraction |
| `dnsx` | DNS enumeration |
| `nuclei` | Template-driven vulnerability + misconfiguration checks |
| `katana` | Web crawler (optional, web-app use cases) |

Nuclei's template repo (`projectdiscovery/nuclei-templates`, 10k+ checks,
actively maintained) is a community asset we'd pin to signed commits and
ship as curated bundles.

## Architecture

### New target type: `network_range`

Config: CIDR, optional port ranges, rate-limit profile. No credentials at
discovery time.

### New bundle runtime: `recon`

The current bundle model has a Python runtime (`content/checks.py`). Add a
second runtime:

```yaml
# bundle manifest
runtime: recon
pipeline:
  - naabu:  --rate 100 --top-ports 100
  - httpx:  -tech-detect -tls-probe -server -version
  - nuclei: -severity critical,high -tags cve,default-login,misconfig
```

The agent executes each stage, passes outputs downstream, and emits a
structured inventory back over the same WSS tunnel.

### New DB schema: `discovered_assets`

```
discovered_assets(
  id, tenant_id, scan_id, scan_run_id,
  ip, hostname?, port, service, version?, technologies[],
  cves[{id, cvss, kev}],
  first_seen, last_seen
)
```

Results from each recon run update this table; the UI shows current state,
the history is queryable for drift detection ("this host grew a new
service since last week").

### Correlation rules

Structured policy — kept simple to start:

```yaml
rules:
  - match: { service: postgresql, version: "16.*" }
    action: create_target
    target_type: database
    bundle: cis-postgresql-16
    require_credential: true

  - match: { service: ssh }
    action: create_target
    target_type: host
    bundle: cis-linux-host
    require_credential: true
```

Two modes: **suggest** (admin approves each) and **auto** (rule fires
without intervention, admin can undo). Default: suggest.

### New UI surfaces

- **Discovered Assets** page — filterable table, per-row "Create target"
  action, CVE badges with CVSS
- **Discovery scans** page — schedule, run, history; separate from
  compliance scans
- **Rules** page under Admin — write/edit correlation rules
- **Dashboard widgets** — "15 new assets this week", "3 new criticals on
  existing assets"

## Market positioning shift

From:
> "CIS compliance scanner for regulated industries"

To:
> "Know what you have, what's vulnerable, and whether it's compliant — from one lightweight agent."

Competitors: Tenable, Rapid7 InsightVM, Qualys, Runecast. All expensive
enterprise tools. An agent-based, OSS-core, smaller-scope version fits
the "quietly powerful" positioning we've already committed to.

## Constraints and caveats

1. **Scanning noise** — port scans on live subnets trip IDS/NAC in some
   shops. Ship a low-and-slow default; expose rate limit + maintenance
   window concepts.
2. **Legal** — customer must authorize scanning their own internal
   network. Put it in onboarding docs + ToS. Default: explicit CIDR
   allowlist, never scan outside it.
3. **Accuracy** — version banners lie; server strings get stripped.
   Treat fingerprints as hints, not ground truth. Compliance still
   requires credentialed access.
4. **Scale** — a /16 is 65k hosts. Schedule per-/24 chunks; give
   scheduling UI first-class treatment.
5. **Vuln DB freshness** — Nuclei templates update fast but not
   instantly. For critical CVEs (log4shell-class events) also pull
   NVD/KEV feeds directly.
6. **Binary size** — the PD stack is ~100-150MB combined. Agent image
   grows; either bundle it or download on demand on first recon run.
7. **False positives** — inevitable. Need per-finding suppression
   (with justification + expiry) and confidence scoring in results.

## Suggested sequencing

Each step shippable on its own; each compounds value.

1. **v0.x** (current): CIS bundles, hand-built targets.
2. **v0.(x+1) — Nuclei as a bundle runtime.** First Nuclei-driven bundle:
   "Web exposure baseline" (~50 curated templates — default creds,
   exposed admin panels, stale SSL, known CVEs on common frameworks).
   Signed by our Ed25519 key. Pinned to a nuclei-templates commit.
3. **v0.(x+2) — Single-CIDR discovery.** New target type + recon
   runtime + Discovered Assets page, read-only.
4. **v0.(x+3) — Promote + manual rules.** "Create target from
   discovered asset" action. Rule engine in suggest mode.
5. **v1.0 — Automation.** Auto-correlation mode, risk scoring, SLAs,
   scheduled rescans, drift alerts.

## What we are not building

- Generic external attack surface management (subfinder, public-internet
  scans). We stay inside customer networks; that's our agent model's
  native fit.
- EDR / endpoint telemetry. Different product.
- A from-scratch vulnerability database. Lean on NVD + Nuclei templates +
  KEV. Our value is the agent + pipeline + compliance integration, not
  the CVE data itself.

## Consequences

- **Positive:** larger TAM, stronger differentiation, each customer
  sees value faster (inventory on day one, not "go build a target list").
- **Negative:** scope grows; more moving parts; compliance team must
  now also understand recon. Mitigation: keep modes explicit in the UI
  — a customer using only compliance never sees the recon pages.
- **Neutral:** product name and site copy will need revisions when we
  ship this. Mostly just stop saying "CIS compliance scanner" in
  isolation.

## Open questions

Resolve before starting implementation:

- Do we ship the PD binaries inside the agent image or download on
  demand? (Leaning: download on first recon run, cache on disk next
  to the compliance bundles.)
- How do we sign discovered-asset data at rest? (Probably just store
  as JSONB; no integrity concern inside our own DB.)
- Do we store raw Nuclei output or only our normalized schema?
  (Normalized + keep raw for 30 days for debugging.)
- Rule language: structured YAML or expression DSL like OPA/Rego?
  Start with simple structured matching; revisit if rules get
  sophisticated.
- Pricing implications: does recon cost more than compliance? For OSS
  core this is a go-to-market question, not a technical one.
