# SilkStrand

SilkStrand is a SaaS-based CIS compliance scanner that reaches into private customer environments via lightweight edge agents. Sensitive data never leaves the customer network — only structured compliance results traverse the tunnel.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                   SilkStrand SaaS (GCP)                 │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │
│  │ React UI │  │  Go API  │  │ Upstash  │  │  GCS   │  │
│  │  (Web)   │──│ (Cloud   │──│  Redis   │  │Bundles │  │
│  │          │  │   Run)   │  │ (pub/sub)│  │        │  │
│  └──────────┘  └────┬─────┘  └────┬─────┘  └───┬────┘  │
│                     │             │             │        │
│                ┌────┴─────┐      │             │        │
│                │ Cloud SQL│      │             │        │
│                │ Postgres │      │             │        │
│                └──────────┘      │             │        │
└──────────────────────────────────┼─────────────┼────────┘
                                   │             │
              ─ ─ ─ ─ ─ ─ ─ WSS 443 (outbound) ─ ─ ─ ─ ─
                                   │             │
┌──────────────────────────────────┼─────────────┼────────┐
│            Customer Environment  │             │        │
│                                  │             │        │
│  ┌───────────────────────────────┴─────────────┴─────┐  │
│  │              SilkStrand Agent (Go binary)         │  │
│  │                                                   │  │
│  │  ┌─────────┐ ┌──────────┐ ┌────────┐ ┌────────┐  │  │
│  │  │ Tunnel  │ │ Runner   │ │ Cache  │ │ Vault  │  │  │
│  │  │ (WSS)   │ │ (polyglot│ │(bundles│ │ Client │  │  │
│  │  │         │ │  exec)   │ │   )    │ │        │  │  │
│  │  └─────────┘ └────┬─────┘ └────────┘ └───┬────┘  │  │
│  └────────────────────┼──────────────────────┼───────┘  │
│                       │                      │          │
│               ┌───────┴───────┐    ┌─────────┴───────┐  │
│               │  Scan Targets │    │  Secret Store   │  │
│               │  (DB, OS, etc)│    │ (Vault, CyberArk│  │
│               └───────────────┘    │  Secrets Mgr)   │  │
│                                    └─────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Agent | Go | Single binary, cross-compilation, goroutines for concurrency |
| API Server | Go | One language across backend, strong stdlib |
| Frontend | React + TypeScript | Rich component ecosystem, standard SPA |
| Database | Cloud SQL PostgreSQL | Managed, reliable, GCP-native |
| Real-time | Upstash Redis | Serverless Redis, pay-per-request, zero idle cost |
| Hosting | GCP Cloud Run | Serverless containers, scale-to-zero |
| Object Storage | GCS | Compliance bundles (signed, versioned) |
| IaC | Terraform | GCP infrastructure management |
| Auth | Auth0 or Clerk | Off-the-shelf, don't build auth |

## Project Structure

```
silkstrand/
├── agent/          # Go edge agent binary
├── api/            # Go API server (Cloud Run)
├── web/            # React + TypeScript frontend
├── terraform/      # GCP infrastructure
├── bundles/        # Compliance bundle specs & examples
└── docs/           # Architecture, user stories, ADRs
```

## Architectural Principles

1. **Data never leaves the customer network** — raw config data stays on-prem. Only structured results (pass/fail, evidence snippets) traverse the tunnel.
2. **Outbound-only connectivity** — agents never require inbound firewall rules. WSS over 443, proxy-compatible.
3. **Credential zero-knowledge** — the SaaS control plane never sees or stores target credentials (post-MVP). Agent fetches from customer vault JIT.
4. **Framework-agnostic execution** — the platform is a polyglot runtime. Bundle authors choose their assessment language (OVAL, Rego, Python, Perl); the output schema is the contract.
5. **Thin agent, smart bundles** — keep the agent simple (tunnel, runner, cache). All compliance logic lives in bundles that can be updated independently.
6. **Cost-minimal by default** — serverless-first (Cloud Run, Upstash). Scale to zero. No always-on infrastructure beyond Cloud SQL.
7. **Single-person sustainability** — favor boring technology, minimal dependencies, one language (Go) on the backend. Every added dependency must justify its ops burden.

## Coding Conventions

### Go (Agent + API)

- Standard `go fmt` and `go vet`
- Use stdlib where possible; minimize third-party dependencies
- Error handling: wrap errors with context using `fmt.Errorf("doing X: %w", err)`
- Logging: structured logging (slog)
- Tests: table-driven tests, test files alongside source
- No global mutable state; pass dependencies via constructors

### React + TypeScript (Web)

- Functional components with hooks
- TypeScript strict mode
- API types generated from Go API definitions where possible
- Component structure: one component per file, colocated styles and tests

### Terraform

- One module per logical service
- Variables for all environment-specific values
- Remote state in GCS backend
- No hardcoded project IDs, regions, or credentials

### General

- Commits: conventional commits format (`feat:`, `fix:`, `docs:`, etc.)
- Branches: `feature/`, `fix/`, `docs/` prefixes
- PRs: require description of what and why
- No secrets in code — use environment variables or secret managers

## Key Design Decisions

- **Upstash Redis over self-hosted Redis**: Eliminates idle cost and ops burden. Serverless pay-per-request model. See `docs/adr/001-upstash-over-redis.md`.
- **Agent runs scans locally**: Data never leaves customer network. Agent is an execution engine, not a proxy.
- **Polyglot bundle system**: Bundles declare their framework in a manifest. Agent selects the appropriate runner. Standardized JSON output schema is the contract.
- **MVP credential model**: Basic auth stored in platform. Post-MVP: integrate with Vault, CyberArk, AWS/GCP Secrets Manager.
- **First benchmark**: CIS PostgreSQL — showcases the authenticated scan differentiator.

## Branching & Deployment

- No direct commits to `main` — all changes via `feature/` or `fix/` branches with PR
- PR triggers CI: lint, test, build verify, Terraform plan (both envs posted as PR comments)
- Merge to `main` auto-deploys to `silkstrand-stage`
- Git tag (`v*`) promotes to `silkstrand-prod` using the same image SHA (no rebuild)
- Agent binary cross-compiled and attached to GitHub Release on tag
- GCP auth via Workload Identity Federation (no service account keys)
- Terraform state in GCS: `gs://silkstrand-{stage,prod}-tfstate/`
- See `docs/cicd.md` for full details

## GCP Projects

| Environment | Project ID | Deploy Trigger |
|-------------|-----------|----------------|
| Stage | `silkstrand-stage` | Auto on merge to `main` |
| Prod | `silkstrand-prod` | Manual via git tag `v*` |
