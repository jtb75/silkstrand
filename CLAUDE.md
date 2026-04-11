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
| Database | Cloud SQL PostgreSQL 16 | Managed, reliable, GCP-native |
| Real-time | Upstash Redis | Serverless Redis, pay-per-request, zero idle cost |
| Hosting | GCP Cloud Run | Serverless containers, scale-to-zero |
| Object Storage | GCS | Compliance bundles (signed, versioned) |
| Container Registry | Artifact Registry | `us-central1-docker.pkg.dev/silkstrand-{env}/silkstrand/` |
| IaC | OpenTofu (Terraform-compatible) | GCP infrastructure management |
| DNS | Cloudflare | DNS management, domain: silkstrand.io |
| Auth | Auth0 or Clerk | Off-the-shelf, don't build auth |

## Project Structure

```
silkstrand/
├── api/                    # Go API server (Cloud Run)
│   ├── cmd/silkstrand-api/ # Entry point
│   ├── internal/
│   │   ├── config/         # Environment-based config
│   │   ├── handler/        # HTTP handlers (health, target, scan, agent)
│   │   ├── middleware/     # Auth (JWT), tenant isolation, logging
│   │   ├── model/          # Domain types
│   │   ├── store/          # Postgres data access + migrations
│   │   ├── pubsub/         # Upstash Redis pub/sub
│   │   └── websocket/     # Agent WebSocket hub
│   └── Dockerfile          # Multi-stage (golang → distroless)
├── agent/                  # Go edge agent binary (not yet built)
├── web/                    # React + TypeScript frontend (not yet built)
├── terraform/
│   ├── bootstrap/          # One-time setup: state buckets, WIF, IAM
│   ├── environments/
│   │   ├── stage/          # silkstrand-stage wiring
│   │   └── prod/           # silkstrand-prod wiring
│   └── modules/
│       ├── networking/     # VPC, private services, VPC connector
│       ├── database/       # Cloud SQL PostgreSQL
│       ├── cloud-run/      # API Cloud Run service
│       ├── storage/        # GCS bundles bucket
│       └── dns/            # Cloudflare DNS records
├── bundles/                # Compliance bundle specs & examples
├── docs/                   # Architecture, user stories, ADRs, CI/CD
├── docker-compose.yml      # Local dev: Postgres (15432) + Redis (16379)
└── Makefile                # dev, build, test, lint, docker commands
```

## Current State

### What's Deployed (Stage)

- **Cloud Run API** — `https://silkstrand-api-uy4v4rttgq-uc.a.run.app`
  - Health: `/healthz`, `/readyz`
  - Targets CRUD: `/api/v1/targets`
  - Scans: `/api/v1/scans`
  - Agent WebSocket: `/ws/agent`
  - JWT auth with tenant isolation
- **Cloud SQL PostgreSQL 16** — `db-f1-micro`, private IP only
- **Upstash Redis** — connected for pub/sub
- **GCS Bucket** — `silkstrand-stage-bundles`
- **VPC** — private services access, serverless VPC connector
- **DNS** — `api-stage.silkstrand.io` → Cloud Run domain mapping (Google-managed TLS)
- **DNS** — `agent-stage.silkstrand.io` → Cloud Run domain mapping

### What's Not Built Yet

- Go edge agent
- React frontend
- Prod deployment (infra defined, not applied)
- Custom compliance bundles
- Vault integrations

## API Routes

```
GET  /healthz                    # Liveness check (no auth)
GET  /readyz                     # Readiness: DB + Redis connectivity

GET  /ws/agent                   # WebSocket for agent connections

GET    /api/v1/targets           # List targets (JWT required)
POST   /api/v1/targets           # Create target
GET    /api/v1/targets/{id}      # Get target
PUT    /api/v1/targets/{id}      # Update target
DELETE /api/v1/targets/{id}      # Delete target

POST   /api/v1/scans             # Trigger scan
GET    /api/v1/scans             # List scans
GET    /api/v1/scans/{id}        # Get scan with results
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

- Go 1.24 (pinned for golangci-lint compatibility)
- Standard `go fmt` and `go vet`
- Use stdlib where possible; minimize third-party dependencies
- Stdlib `net/http` router (Go 1.22+ enhanced routing, no gorilla/mux)
- Error handling: wrap errors with context using `fmt.Errorf("doing X: %w", err)`
- Logging: structured logging (slog, JSON output)
- Tests: table-driven tests, test files alongside source
- No global mutable state; pass dependencies via constructors
- Dependencies: pgx (Postgres), gorilla/websocket, go-redis, golang-migrate

### React + TypeScript (Web)

- Functional components with hooks
- TypeScript strict mode
- API types generated from Go API definitions where possible
- Component structure: one component per file, colocated styles and tests

### Terraform / OpenTofu

- One module per logical service
- Variables for all environment-specific values
- Remote state in GCS backend
- No hardcoded project IDs, regions, or credentials
- Use `tofu` CLI locally (OpenTofu), `terraform` in CI (hashicorp/setup-terraform action)

### General

- Commits: conventional commits format (`feat:`, `fix:`, `docs:`, etc.)
- Branches: `feature/`, `fix/`, `docs/` prefixes
- PRs: require description of what and why
- No secrets in code — use environment variables or Secret Manager

## Key Design Decisions

- **Upstash Redis over self-hosted Redis**: Eliminates idle cost and ops burden. Serverless pay-per-request model. See `docs/adr/001-upstash-over-redis.md`.
- **Agent runs scans locally**: Data never leaves customer network. Agent is an execution engine, not a proxy.
- **Polyglot bundle system**: Bundles declare their framework in a manifest. Agent selects the appropriate runner. Standardized JSON output schema is the contract.
- **Artifact Registry over GHCR**: Cloud Run requires images from GCR, Artifact Registry, or Docker Hub. Images at `us-central1-docker.pkg.dev/silkstrand-{env}/silkstrand/`.
- **Cloud Run domain mapping**: Custom domains use `ghs.googlehosted.com` CNAME with Google-managed TLS, not Cloudflare proxy.
- **Cloud SQL private IP only**: No public IP. Cloud Run reaches DB via Serverless VPC Access connector.
- **MVP credential model**: Basic auth stored in platform. Post-MVP: integrate with Vault, CyberArk, AWS/GCP Secrets Manager.
- **First benchmark**: CIS PostgreSQL — showcases the authenticated scan differentiator.

## Branching & Deployment

- No direct commits to `main` — all changes via `feature/` or `fix/` branches with PR
- PR triggers CI: lint, test, build verify, Terraform plan (both envs posted as PR comments)
- CI uses path-based filtering: Go/web/terraform/docker jobs only run when relevant files change
- Merge to `main` auto-deploys to `silkstrand-stage` (build image → Artifact Registry → Cloud Run)
- Git tag (`v*`) promotes to `silkstrand-prod`
- Agent binary cross-compiled and attached to GitHub Release on tag
- GCP auth via Workload Identity Federation (no service account keys)
- Terraform state in GCS: `gs://silkstrand-{stage,prod}-tfstate/`
- See `docs/cicd.md` for full details

## GCP Projects

| Environment | Project ID | Deploy Trigger | Cloud Run URL |
|-------------|-----------|----------------|---------------|
| Stage | `silkstrand-stage` | Auto on merge to `main` | `silkstrand-api-uy4v4rttgq-uc.a.run.app` |
| Prod | `silkstrand-prod` | Manual via git tag `v*` | Not deployed yet |

## Local Development

```bash
make dev          # Start Postgres (15432) + Redis (16379), run API
make build        # Build API binary
make test         # Run tests
make lint         # Run golangci-lint
make docker       # Build Docker image
make down         # Stop containers
make clean        # Stop containers + delete volumes
```

Ports 5432 and 6379 are likely in use locally, so docker-compose maps to 15432 and 16379.

## GitHub Secrets & Variables

### Secrets
- `CLOUDFLARE_API_TOKEN` — DNS management for silkstrand.io
- `UPSTASH_REDIS_URL_STAGE` / `UPSTASH_REDIS_URL_PROD` — Redis connection URLs
- `JWT_SECRET_STAGE` / `JWT_SECRET_PROD` — API JWT signing keys

### Variables
- `WIF_PROVIDER_STAGE` / `WIF_PROVIDER_PROD` — Workload Identity Federation provider names
- `WIF_SA_STAGE` / `WIF_SA_PROD` — GitHub Actions service account emails
- `CLOUDFLARE_ZONE_ID` — Zone ID for silkstrand.io
