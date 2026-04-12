# SilkStrand

SilkStrand is a SaaS-based CIS compliance scanner that reaches into private customer environments via lightweight edge agents. Sensitive data never leaves the customer network — only structured compliance results traverse the tunnel.

## Architecture Overview

SilkStrand has a three-tier architecture: a **backoffice manager** (control plane), one or more **data centers** (regional deployments), and **edge agents** (customer environments).

```
┌─────────────────────────────────────────────────────────────┐
│            Backoffice Manager (own GCP project)             │
│                                                             │
│  ┌──────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ React UI │  │ Go API       │  │ Cloud SQL Postgres    │  │
│  │ (Admin)  │──│ (Cloud Run)  │──│ (DCs, tenants, admin) │  │
│  └──────────┘  └──────┬───────┘  └──────────────────────┘  │
└────────────────────────┼────────────────────────────────────┘
                         │ HTTPS (/internal/v1/)
        ┌────────────────┼────────────────┐
        ▼                ▼                ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  DC: US      │  │  DC: EU      │  │  DC: APAC    │
│  (us-central)│  │  (eu-west)   │  │  (future)    │
└──────┬───────┘  └──────────────┘  └──────────────┘
       │
┌──────┴──────────────────────────────────────────────┐
│              Data Center (per-region GCP project)    │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌────────┐ ┌───────┐  │
│  │ React UI │  │  Go API  │  │Upstash │ │  GCS  │  │
│  │ (Tenant) │──│ (Cloud   │──│ Redis  │ │Bundles│  │
│  │          │  │   Run)   │  │(pub/sub│ │       │  │
│  └──────────┘  └────┬─────┘  └────────┘ └───┬───┘  │
│                     │                        │      │
│                ┌────┴─────┐                  │      │
│                │ Cloud SQL│                  │      │
│                │ Postgres │                  │      │
│                └──────────┘                  │      │
└──────────────────────┬───────────────────────┼──────┘
                       │                       │
          ─ ─ ─ ─  WSS 443 (outbound) ─ ─ ─ ─ ─
                       │                       │
┌──────────────────────┼───────────────────────┼──────┐
│   Customer Environment                       │      │
│  ┌───────────────────┴───────────────────────┴───┐  │
│  │           SilkStrand Agent (Go binary)        │  │
│  │  ┌────────┐ ┌────────┐ ┌───────┐ ┌────────┐  │  │
│  │  │ Tunnel │ │ Runner │ │ Cache │ │ Vault  │  │  │
│  │  │ (WSS)  │ │(Python)│ │(local)│ │ Client │  │  │
│  │  └────────┘ └───┬────┘ └───────┘ └───┬────┘  │  │
│  └─────────────────┼────────────────────┼────────┘  │
│             ┌──────┴──────┐    ┌────────┴────────┐  │
│             │ Scan Targets│    │  Secret Store    │  │
│             │ (DB, OS)    │    │ (Vault, CyberArk)│  │
│             └─────────────┘    └─────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### Key Driver: Data Residency

Each data center is a full deployment in a specific region. EU customers get their data in an EU data center. The backoffice provides cross-datacenter visibility and management without accessing DC databases directly.

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Agent | Go | Single binary, cross-compilation, goroutines for concurrency |
| DC API Server | Go | One language across backend, strong stdlib |
| Backoffice API | Go | Same patterns as DC API, separate deployment |
| Tenant Frontend | React + TypeScript | Rich component ecosystem, standard SPA |
| Backoffice Frontend | React + TypeScript | Same stack, separate app, navy/teal theme |
| Database | Cloud SQL PostgreSQL 16 | Managed, reliable, GCP-native |
| Real-time | Upstash Redis | Serverless Redis, pay-per-request, zero idle cost |
| Hosting | GCP Cloud Run | Serverless containers, scale-to-zero |
| Object Storage | GCS | Compliance bundles (signed, versioned) |
| Container Registry | Artifact Registry | `us-central1-docker.pkg.dev/silkstrand-{env}/silkstrand/` |
| IaC | OpenTofu (Terraform-compatible) | GCP infrastructure management |
| DNS | Cloudflare | DNS management, domain: silkstrand.io |
| Auth | Clerk (planned) | Off-the-shelf tenant auth. Backoffice uses own JWT + bcrypt. |

## Project Structure

```
silkstrand/
├── api/                    # Data Center Go API server (Cloud Run)
│   ├── cmd/silkstrand-api/ # Entry point
│   ├── internal/
│   │   ├── config/         # Environment-based config
│   │   ├── crypto/         # AES-256-GCM for credential encryption
│   │   ├── handler/        # HTTP handlers (health, target, scan, agent, internal)
│   │   ├── middleware/     # Auth (JWT), tenant isolation, internal API key, logging
│   │   ├── model/          # Domain types
│   │   ├── store/          # Postgres data access + migrations
│   │   ├── pubsub/         # Upstash Redis pub/sub
│   │   └── websocket/     # Agent WebSocket hub + message types
│   └── Dockerfile
├── agent/                  # Go edge agent binary
│   ├── cmd/silkstrand-agent/
│   └── internal/
│       ├── config/         # Agent configuration (env vars)
│       ├── tunnel/         # WSS connection, reconnect, message types
│       ├── runner/         # Python runner, manifest parser
│       └── cache/          # Local bundle cache
├── backoffice/             # Backoffice Manager (separate deployment)
│   ├── cmd/backoffice-api/ # Entry point
│   ├── internal/
│   │   ├── config/         # Config (port 8081, own DB on 15433)
│   │   ├── crypto/         # AES-256-GCM for DC API key encryption
│   │   ├── dcclient/       # HTTP client for DC internal API
│   │   ├── handler/        # Datacenter, tenant, health, auth handlers
│   │   ├── middleware/     # Admin JWT auth, role-based access, logging
│   │   ├── model/          # Backoffice domain types
│   │   └── store/          # Postgres data access + migrations
│   └── web/                # Backoffice React frontend (navy/teal theme)
│       └── src/
│           ├── api/        # API client + types
│           ├── components/ # Layout, StatusBadge, DataCenterCard
│           └── pages/      # Login, Dashboard, DataCenters, Tenants
├── web/                    # Tenant React frontend
│   └── src/
│       ├── api/            # API client + types
│       ├── components/     # Layout, TokenPrompt
│       └── pages/          # Dashboard, Targets, Scans, ScanResults
├── bundles/                # Compliance bundles
│   └── cis-postgresql-16/  # First bundle: 8 CIS PostgreSQL controls
│       ├── manifest.yaml
│       ├── content/checks.py
│       └── seed.sql
├── terraform/
│   ├── bootstrap/
│   ├── environments/
│   │   ├── stage/
│   │   └── prod/
│   └── modules/
│       ├── networking/
│       ├── database/
│       ├── cloud-run/
│       ├── storage/
│       └── dns/
├── docs/                   # Architecture, user stories, ADRs, CI/CD
├── docker-compose.yml      # Local dev: Postgres (15432), Redis (16379), Backoffice Postgres (15433)
└── Makefile
```

## Current State

### What's Built

- **Data Center API** — Full Go API server with:
  - User-facing routes: targets CRUD, scans, scan results (JWT + tenant middleware)
  - Dual-mode auth: Clerk JWKS (RS256, production) + HMAC-SHA256 (dev). Stdlib-only crypto.
  - Agent WebSocket endpoint with per-agent API key auth (SHA-256, dual-key rotation)
  - Internal API routes (`/internal/v1/`) for backoffice access (API key auth)
  - Tenant status enforcement (active/suspended/inactive with 5s TTL cache)
  - Scan lifecycle: create → directive via Upstash Redis → agent executes → results via WSS → stored in Postgres
  - Credential encryption at rest (AES-256-GCM), decrypted before forwarding to agents
  - Stuck scan cleanup: running scans fail automatically on agent disconnect
  - Dockerfile: multi-stage (golang:1.24-alpine → distroless)
- **Edge Agent** — Go binary with WSS tunnel (exponential backoff reconnect), Python runner, bundle cache, heartbeat. Cross-compiled for 6 platforms on release.
- **CIS PostgreSQL Bundle** — 8 CIS Benchmark controls (log_connections, ssl, password_encryption, pg_hba.conf, log_statement, pgaudit, superuser roles). End-to-end tested locally.
- **Tenant Frontend** — React + TypeScript SPA: dashboard, targets CRUD, scan triggering, results viewer with summary bar. Clerk integration (production) + dev token flow (local). Dockerfile with nginx.
- **Backoffice Manager** — Separate Go module + React frontend:
  - Data center registration with AES-256-GCM encrypted API key storage
  - Two-phase tenant provisioning (backoffice DB → DC API call, retry on failure)
  - Tenant suspend/activate with DC sync
  - Health poller (60s) monitors all registered data centers
  - Admin JWT auth with role-based access (viewer/admin/super_admin) + bcrypt login
  - Dashboard with DC health cards, cross-DC tenant management
  - Dockerfile: multi-stage (golang → distroless for API, node → nginx for web)
- **CI/CD** — GitHub Actions with path-based filtering for all components (DC API, agent, tenant web, backoffice API, backoffice web, Terraform). Docker image verify builds. Smoke test with fallback.
- **Seed Tooling** — SQL seeds for DC + backoffice databases, runner script, JWT generator, bcrypt hash helper. `make seed`, `make jwt`.
- **Terraform** — Backoffice services (API + web Cloud Run, second database) defined in prod environment alongside DC services. Shared Cloud SQL instance.

### What's Deployed (Stage)

- **Cloud Run API** — `https://silkstrand-api-uy4v4rttgq-uc.a.run.app`
- **Cloud SQL PostgreSQL 16** — `db-f1-micro`, private IP only
- **Upstash Redis** — connected for pub/sub
- **GCS Bucket** — `silkstrand-stage-bundles`
- **VPC** — private services access, serverless VPC connector
- **DNS** — `api-stage.silkstrand.io`, `agent-stage.silkstrand.io`

### What's Not Deployed Yet

- Prod deployment (Terraform defined but not applied — needs `tofu apply` with secrets)
- Backoffice Cloud Run services (Terraform in prod/main.tf, images not yet pushed)
- Stage Cloud Run service crashes on startup (needs DB migration investigation in GCP console)

### What's Not Built Yet

- GCS bundle pull (agent reads local filesystem only — production needs GCS download)
- Bundle upload API (bundles registered in DB but no upload endpoint)
- Additional compliance bundles (only CIS PostgreSQL 16 exists)
- Vault/CyberArk credential integrations (post-MVP — currently AES-256-GCM at rest)
- Tenant-configurable credential sources (each tenant will need pluggable integrations)
- Frontend pagination for list endpoints
- Agent WebSocket CORS restriction for production

## DC API Routes

```
# Public (no auth)
GET  /healthz                              # Liveness
GET  /readyz                               # Readiness: DB + Redis

# Agent WebSocket (per-agent key auth)
GET  /ws/agent?agent_id={id}               # Authorization: Bearer {agent_key}

# Internal (X-API-Key auth, for backoffice)
POST   /internal/v1/tenants                # Create tenant
GET    /internal/v1/tenants                # List all tenants
GET    /internal/v1/tenants/{id}           # Get tenant
PUT    /internal/v1/tenants/{id}           # Update tenant (name, status, config)
DELETE /internal/v1/tenants/{id}           # Soft delete (set inactive)
GET    /internal/v1/agents                 # List all agents (cross-tenant)
GET    /internal/v1/stats                  # DC aggregate stats
POST   /internal/v1/credentials            # Store encrypted credential

# Tenant API (JWT + tenant middleware)
GET    /api/v1/targets                     # List targets
POST   /api/v1/targets                     # Create target
GET    /api/v1/targets/{id}                # Get target
PUT    /api/v1/targets/{id}                # Update target
DELETE /api/v1/targets/{id}                # Delete target
POST   /api/v1/scans                       # Trigger scan
GET    /api/v1/scans                       # List scans
GET    /api/v1/scans/{id}                  # Get scan with results
```

## Backoffice API Routes

```
# Public (no auth)
GET  /healthz                              # Liveness
GET  /readyz                               # Readiness: DB
POST /api/v1/auth/login                    # Admin login (email + password → JWT)

# Admin (JWT with role-based access)
GET    /api/v1/dashboard                   # Aggregate stats from all DCs
POST   /api/v1/data-centers                # Register DC (name, region, url, api_key)
GET    /api/v1/data-centers                # List DCs with health status
GET    /api/v1/data-centers/{id}           # DC detail (optional ?stats=true)
PUT    /api/v1/data-centers/{id}           # Update DC
DELETE /api/v1/data-centers/{id}           # Soft delete DC
POST   /api/v1/tenants                     # Create tenant (two-phase provisioning)
GET    /api/v1/tenants                     # List tenants (optional ?data_center_id=)
GET    /api/v1/tenants/{id}                # Get tenant
PUT    /api/v1/tenants/{id}                # Update tenant
PUT    /api/v1/tenants/{id}/status         # Toggle active/suspended
POST   /api/v1/tenants/{id}/retry          # Retry failed provisioning
```

## WebSocket Protocol

Agent-to-API messages use `{"type": "string", "payload": {...}}` envelope.

| Type | Direction | Payload |
|------|-----------|---------|
| `directive` | server → agent | scan_id, bundle_id/name/version, target_id/type/identifier/config, credentials |
| `scan_started` | agent → server | scan_id |
| `scan_results` | agent → server | scan_id, results (standard schema) |
| `scan_error` | agent → server | scan_id, error |
| `heartbeat` | agent → server | version, uptime_seconds |

Server sends WebSocket pings every 30s; agent responds with pong (60s timeout).

## Architectural Principles

1. **Data never leaves the customer network** — raw config data stays on-prem. Only structured results (pass/fail, evidence snippets) traverse the tunnel.
2. **Data residency** — each data center is a regional deployment. EU data stays in EU. Backoffice manages across DCs without direct DB access.
3. **Outbound-only connectivity** — agents never require inbound firewall rules. WSS over 443, proxy-compatible.
4. **Credential encryption at rest** — MVP: AES-256-GCM in DC database, decrypted before sending to agent over TLS. Post-MVP: agent fetches from customer vault JIT.
5. **Framework-agnostic execution** — polyglot bundle runtime. Bundle authors choose their assessment language; standardized JSON output schema is the contract.
6. **Thin agent, smart bundles** — agent is tunnel + runner + cache. All compliance logic lives in updateable bundles.
7. **Cost-minimal by default** — serverless-first (Cloud Run, Upstash). Scale to zero. No always-on infrastructure beyond Cloud SQL.
8. **Single-person sustainability** — boring technology, minimal dependencies, one language (Go) on the backend.

## Coding Conventions

### Go (Agent + API + Backoffice)

- Go 1.24 (pinned for golangci-lint compatibility)
- Standard `go fmt` and `go vet`
- Use stdlib where possible; minimize third-party dependencies
- Stdlib `net/http` router (Go 1.22+ enhanced routing, no gorilla/mux)
- Error handling: wrap errors with context using `fmt.Errorf("doing X: %w", err)`
- Logging: structured logging (slog, JSON output)
- Tests: table-driven tests, test files alongside source
- No global mutable state; pass dependencies via constructors
- DC API deps: pgx (Postgres), gorilla/websocket, go-redis, golang-migrate
- Agent deps: gorilla/websocket, gopkg.in/yaml.v3
- Backoffice deps: pgx, golang-migrate, golang.org/x/crypto (bcrypt)

### React + TypeScript (Web + Backoffice Web)

- Functional components with hooks
- TypeScript strict mode
- Plain CSS (no framework)
- One component per file
- @tanstack/react-query for data fetching
- Always run `tsc --noEmit` and `eslint` before committing

### Terraform / OpenTofu

- One module per logical service
- Variables for all environment-specific values
- Remote state in GCS backend
- No hardcoded project IDs, regions, or credentials

### General

- Commits: conventional commits format (`feat:`, `fix:`, `docs:`, etc.)
- Branches: `feature/`, `fix/`, `docs/` prefixes
- PRs: require description of what and why
- No secrets in code — use environment variables or Secret Manager

## Key Design Decisions

- **Per-agent API keys**: Each agent gets a unique 256-bit key (SHA-256 hashed in DB). Dual-key rotation via `key_hash` + `next_key_hash`. Constant-time comparison.
- **Tenant status enforcement**: Middleware checks tenant status with 5s TTL cache. Suspended tenants get 403 on all API routes and agent WSS connections.
- **Backoffice in prod project**: Runs as additional Cloud Run services in `silkstrand-prod` — not a separate GCP project. Uses second database on the same Cloud SQL instance ($0 extra). One backoffice manages all DCs (stage, prod, future regions).
- **Two-phase tenant provisioning**: Create in backoffice DB first (provisioning_status=pending), then call DC API. On failure, mark as failed with retry option. Returns 202 (not 201) on DC provisioning failure.
- **Credential encryption at rest**: AES-256-GCM with `CREDENTIAL_ENCRYPTION_KEY` env var. DC API decrypts before sending to agent. No encryption key = passthrough (dev only). Post-MVP: tenant-configurable credential sources (Vault, CyberArk, etc.).
- **Dual-mode auth**: DC API supports both Clerk JWKS (RS256, production) and HMAC-SHA256 (dev). Controlled by `CLERK_JWKS_URL` env var — empty = dev mode. All JWKS crypto is stdlib-only (no external JWT libraries).
- **Stuck scan cleanup**: Running/pending scans automatically fail when agent disconnects.
- **Upstash Redis over self-hosted Redis**: Eliminates idle cost. See `docs/adr/001-upstash-over-redis.md`.
- **Artifact Registry over GHCR**: Cloud Run compatibility. Images at `us-central1-docker.pkg.dev/silkstrand-{env}/silkstrand/`.
- **Cloud Run domain mapping**: Custom domains use `ghs.googlehosted.com` CNAME with Google-managed TLS.
- **Cloud SQL private IP only**: Cloud Run reaches DB via Serverless VPC Access connector.
- **First benchmark**: CIS PostgreSQL 16 — 8 controls showcasing the authenticated scan pipeline.

## Branching & Deployment

- No direct commits to `main` — all changes via `feature/` or `fix/` branches with PR
- PR triggers CI: lint, test, build verify, Terraform plan
- CI uses path-based filtering: Go/web/terraform/docker jobs only run when relevant files change
- Merge to `main` auto-deploys to `silkstrand-stage`
- Git tag (`v*`) promotes to `silkstrand-prod`
- Agent binary cross-compiled and attached to GitHub Release on tag
- GCP auth via Workload Identity Federation (no service account keys)
- Terraform state in GCS: `gs://silkstrand-{stage,prod}-tfstate/`
- See `docs/cicd.md` for full details

## GCP Projects

| Environment | Project ID | Deploy Trigger | Purpose |
|-------------|-----------|----------------|---------|
| Stage | `silkstrand-stage` | Auto on merge to `main` | DC stage deployment |
| Prod | `silkstrand-prod` | Manual via git tag `v*` | DC prod + backoffice (one backoffice manages all DCs) |

## Local Development

```bash
# 1. Start dependencies
make dev-deps         # Start Postgres (15432) + Redis (16379) + Backoffice Postgres (15433)

# 2. Seed test data (idempotent, safe to run multiple times)
make seed             # Creates: test tenant, agent (key: test-agent-key), target, bundle, admin user

# 3. Run services
make run              # DC API on port 8080
make run-backoffice   # Backoffice API on port 8081

# 4. Run agent
cd agent && SILKSTRAND_AGENT_ID=00000000-0000-0000-0000-000000000010 \
  SILKSTRAND_AGENT_KEY=test-agent-key \
  SILKSTRAND_BUNDLE_DIR=../bundles \
  go run ./cmd/silkstrand-agent/

# 5. Run frontends
cd web && npm run dev              # Tenant UI on port 5173 (proxies to localhost:8080)
cd backoffice/web && npm run dev   # Backoffice UI on port 5174 (proxies to localhost:8081)

# 6. Generate a dev JWT for API testing
make jwt              # Prints a valid JWT token for curl
# Usage: curl -H "Authorization: Bearer $(make jwt)" localhost:8080/api/v1/targets

# Build & test
make build            # Build DC API binary
make test             # Run DC API tests
make lint             # Run golangci-lint on DC API
make build-backoffice # Build backoffice binary
make test-backoffice  # Run backoffice tests

# Docker (full stack)
docker compose up --build   # Builds and runs all services (API, backoffice, frontends, DBs, Redis)

# Cleanup
make down             # Stop containers
make clean            # Stop containers + delete volumes
```

### Quick E2E Test

```bash
make dev-deps && make seed
# Terminal 1: make run
# Terminal 2: cd agent && SILKSTRAND_AGENT_ID=00000000-0000-0000-0000-000000000010 SILKSTRAND_AGENT_KEY=test-agent-key SILKSTRAND_BUNDLE_DIR=../bundles go run ./cmd/silkstrand-agent/
# Terminal 3:
TOKEN=$(make jwt)
curl -s -X POST localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"target_id":"00000000-0000-0000-0000-000000000020","bundle_id":"00000000-0000-0000-0000-000000000030"}'
# Wait 1-2 seconds, then:
curl -s localhost:8080/api/v1/scans/<scan_id> -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

### Ports

| Service | Port | Purpose |
|---------|------|---------|
| DC API | 8080 | Data center API server |
| Backoffice API | 8081 | Backoffice API server |
| Postgres (DC) | 15432 | DC database |
| Postgres (Backoffice) | 15433 | Backoffice database |
| Redis | 16379 | Pub/sub for scan directives |

## Environment Variables

### DC API

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |
| `DATABASE_URL` | `postgres://...localhost:5432/silkstrand` | Postgres connection |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection |
| `JWT_SECRET` | `dev-secret-change-in-production` | JWT signing key |
| `INTERNAL_API_KEY` | (none) | API key for backoffice access |
| `CREDENTIAL_ENCRYPTION_KEY` | (none) | 64 hex chars (32 bytes) for AES-256-GCM |
| `CLERK_JWKS_URL` | (none) | Clerk JWKS endpoint for production auth (empty = dev HMAC mode) |

### Backoffice API

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8081` | Server port |
| `DATABASE_URL` | `postgres://...localhost:15433/silkstrand_backoffice` | Postgres connection |
| `JWT_SECRET` | `dev-secret-change-in-production` | Admin JWT signing key |
| `ENCRYPTION_KEY` | (none) | 64 hex chars for DC API key encryption |

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `SILKSTRAND_API_URL` | `ws://localhost:8080` | DC API WebSocket URL |
| `SILKSTRAND_AGENT_ID` | (required) | Agent UUID |
| `SILKSTRAND_AGENT_KEY` | (required) | Agent API key |
| `SILKSTRAND_BUNDLE_DIR` | `./bundles` | Local bundle directory |
| `SILKSTRAND_LOG_LEVEL` | `info` | Log level |

## GitHub Secrets & Variables

### Secrets
- `CLOUDFLARE_API_TOKEN` — DNS management for silkstrand.io
- `UPSTASH_REDIS_URL_STAGE` / `UPSTASH_REDIS_URL_PROD` — Upstash Redis connection URLs (DC API only)
- `JWT_SECRET_STAGE` / `JWT_SECRET_PROD` — DC API JWT signing keys
- `INTERNAL_API_KEY_PROD` — API key for backoffice → DC internal API access
- `BACKOFFICE_JWT_SECRET` — Backoffice admin JWT signing key
- `BACKOFFICE_ENCRYPTION_KEY` — AES-256 key for DC API key encryption in backoffice DB (64 hex chars)
- `CREDENTIAL_ENCRYPTION_KEY_STAGE` / `CREDENTIAL_ENCRYPTION_KEY_PROD` — AES-256 key for credential encryption

### Variables
- `WIF_PROVIDER_STAGE` / `WIF_PROVIDER_PROD` — Workload Identity Federation provider names
- `WIF_SA_STAGE` / `WIF_SA_PROD` — GitHub Actions service account emails
- `CLOUDFLARE_ZONE_ID` — Zone ID for silkstrand.io
- `CLERK_JWKS_URL` — Clerk JWKS endpoint (set when Clerk is configured)
- `VITE_CLERK_PUBLISHABLE_KEY` — Clerk publishable key for tenant frontend build

## Database Migrations

### DC API (`api/internal/store/migrations/`)

| Migration | Description |
|-----------|-------------|
| 001_initial | tenants, agents, targets, credentials, bundles, scans, scan_results |
| 002_agent_auth | key_hash, next_key_hash, key_rotated_at on agents |
| 003_tenant_status | status, config on tenants |

### Backoffice (`backoffice/internal/store/migrations/`)

| Migration | Description |
|-----------|-------------|
| 001_initial | data_centers, tenants, admin_users |
