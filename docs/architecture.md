# SilkStrand Architecture

## System Topology

```
                    ┌─────────────────────────────────────┐
                    │         SilkStrand SaaS (GCP)       │
                    │                                     │
  Users ──HTTPS──▶  │  Cloud Run     Cloud Run            │
                    │  ┌────────┐   ┌────────────┐        │
                    │  │  Web   │   │   API      │        │
                    │  │ (React)│──▶│  (Go)      │        │
                    │  └────────┘   └─────┬──────┘        │
                    │                     │               │
                    │          ┌──────────┼──────────┐    │
                    │          │          │          │    │
                    │    ┌─────┴──┐ ┌─────┴──┐ ┌────┴─┐  │
                    │    │Upstash │ │Cloud   │ │ GCS  │  │
                    │    │ Redis  │ │SQL PG  │ │      │  │
                    │    └────────┘ └────────┘ └──────┘  │
                    └──────────┬──────────────────┬──────┘
                               │                  │
                          WSS 443            HTTPS (GCS)
                          (outbound)         (bundle pull)
                               │                  │
  ┌────────────────────────────┼──────────────────┼──────┐
  │  Customer Environment      │                  │      │
  │                    ┌───────┴──────────────────┴───┐  │
  │                    │     SilkStrand Agent         │  │
  │                    └───────┬─────────────────┬────┘  │
  │                            │                 │       │
  │                    ┌───────┴──┐      ┌───────┴────┐  │
  │                    │  Targets │      │Secret Store│  │
  │                    │(DB, OS,  │      │(Vault, etc)│  │
  │                    │ Cloud)   │      └────────────┘  │
  │                    └──────────┘                      │
  └──────────────────────────────────────────────────────┘
```

## Components

### Edge Agent

A single Go binary deployed in the customer's environment. Responsibilities:

- **Tunnel**: Establishes outbound WSS connection to the SaaS control plane over port 443. Maintains heartbeat. Reconnects with exponential backoff on disconnect. Supports HTTP proxy for corporate environments.
- **Runner**: Polyglot execution engine. Reads bundle manifest, selects appropriate runner (Python, OVAL/OpenSCAP, Rego/OPA, Perl), shells out to execute, captures stdout.
- **Cache**: Local cache of compliance bundles pulled from GCS. Verifies bundle signatures before execution. Invalidates on version change.
- **Vault Client**: Fetches credentials JIT from customer's secret store. MVP: receives basic auth in directive. Post-MVP: integrates with HashiCorp Vault, CyberArk, AWS Secrets Manager, GCP Secret Manager.

Agent internal structure:

```
agent/
├── cmd/
│   └── silkstrand-agent/
│       └── main.go
├── internal/
│   ├── tunnel/       # WSS connection management
│   ├── runner/       # Framework runners
│   │   ├── runner.go # Runner interface
│   │   ├── python.go
│   │   ├── oval.go
│   │   ├── rego.go
│   │   └── perl.go
│   ├── cache/        # Bundle caching & verification
│   └── vault/        # Secret store integrations
├── go.mod
└── go.sum
```

### API Server

Go HTTP server deployed on Cloud Run. Responsibilities:

- REST API for the web frontend (targets, scans, results, users)
- WebSocket endpoint for agent connections
- Publishes directives to Upstash Redis for agent delivery
- Subscribes to Upstash Redis for scan results
- Manages bundle uploads to GCS
- Tenant isolation via middleware

API internal structure:

```
api/
├── cmd/
│   └── silkstrand-api/
│       └── main.go
├── internal/
│   ├── handler/      # HTTP handlers
│   ├── middleware/    # Auth, tenant isolation, logging
│   ├── model/        # Domain models
│   ├── store/        # Postgres data access
│   ├── pubsub/       # Upstash Redis pub/sub
│   └── bundle/       # GCS bundle management
├── go.mod
└── go.sum
```

### Web Frontend

React + TypeScript SPA. Deployed as static assets on Cloud Run (or GCS + CDN).

- Dashboard: agent status, recent scans, compliance posture
- Target management: CRUD targets, associate credentials
- Scan management: trigger scans, view progress, review results
- Bundle management: upload bundles, view versions
- Settings: tenant config, user management

### Infrastructure (Terraform)

```
terraform/
├── main.tf
├── variables.tf
├── outputs.tf
├── modules/
│   ├── cloud-run/      # API + Web services
│   ├── cloud-sql/      # PostgreSQL instance
│   ├── gcs/            # Bundle storage bucket
│   ├── networking/     # VPC, firewall rules
│   └── iam/            # Service accounts, roles
└── environments/
    ├── dev/
    └── prod/
```

## Data Flow: Scan Lifecycle

```
1. User triggers scan via Web UI
   │
   ▼
2. API creates scan record in Postgres (status: PENDING)
   │
   ▼
3. API publishes directive to Upstash Redis
   channel: agent:{agent_id}:directives
   payload: { scan_id, bundle_id, bundle_version, target, credential_ref }
   │
   ▼
4. API server holding agent's WSS connection receives Redis message
   │
   ▼
5. API forwards directive to agent over WSS
   │
   ▼
6. Agent receives directive
   ├── Pulls bundle from GCS (if not cached or version changed)
   ├── Verifies bundle signature
   ├── Reads manifest to determine framework
   ├── Fetches credentials (basic auth from directive MVP, vault post-MVP)
   │
   ▼
7. Agent executes bundle via appropriate runner
   ├── Sets up environment (creds via env vars, target connection info)
   ├── Shells out to framework (python3, oscap, opa, perl)
   ├── Captures stdout
   ├── Parses output to standard results schema
   │
   ▼
8. Agent sends results back over WSS
   │
   ▼
9. API receives results
   ├── Stores in Postgres (scan results, individual control findings)
   ├── Updates scan status: COMPLETED or FAILED
   ├── Publishes status update via Upstash Redis for real-time UI
   │
   ▼
10. Web UI receives update, displays results
```

## Bundle Format

Bundles are packaged as `.tar.gz` archives with a standard structure:

```
cis-postgresql-16-v1.0.0.tar.gz
├── manifest.yaml
├── content/
│   ├── checks.py          # or checks.oval.xml, checks.rego, etc.
│   └── ...
└── README.md              # optional
```

### Manifest Schema

```yaml
name: cis-postgresql-16
version: 1.0.0
framework: python          # python | oval | rego | perl
runtime_version: ">=3.11"  # required runtime version
target_type: database      # database | os | cloud | network
benchmark:
  name: "CIS PostgreSQL 16 Benchmark"
  version: "1.0.0"
  cis_id: "CIS_PostgreSQL_16"
entrypoint: content/checks.py
inputs:
  - name: db_credential
    type: credential
    description: "Database login credential"
  - name: connection
    type: connection
    params: [host, port, database]
outputs:
  format: silkstrand-v1    # standard results schema version
```

## Standard Results Schema

All runners must produce output conforming to this schema:

```json
{
  "schema_version": "1",
  "bundle": {
    "name": "cis-postgresql-16",
    "version": "1.0.0"
  },
  "target": {
    "type": "database",
    "identifier": "10.0.1.50:5432/production"
  },
  "started_at": "2026-04-11T14:30:00Z",
  "completed_at": "2026-04-11T14:30:45Z",
  "status": "completed",
  "summary": {
    "total": 53,
    "pass": 42,
    "fail": 7,
    "error": 1,
    "not_applicable": 3
  },
  "controls": [
    {
      "id": "1.3",
      "title": "Ensure login via host TCP/IP connections is configured correctly",
      "description": "PostgreSQL can use host-based authentication...",
      "status": "FAIL",
      "severity": "HIGH",
      "evidence": {
        "current_value": "trust",
        "expected_value": "scram-sha-256",
        "source": "pg_hba.conf line 92"
      },
      "remediation": "Edit pg_hba.conf and change the authentication method from 'trust' to 'scram-sha-256' for all host entries."
    }
  ]
}
```

## Security Model

### Agent-to-SaaS Communication

- All communication over TLS 1.3 (WSS)
- Agent authenticates to SaaS using a registration token (generated during agent setup)
- After initial registration, agent receives a client certificate for mutual TLS (mTLS) — post-MVP
- WSS connection carries a JWT for ongoing authentication

### Credential Handling

**MVP**: Basic auth credentials (username/password) stored encrypted in Postgres, sent to agent in the scan directive. Simple but not ideal for production.

**Post-MVP**: Zero-knowledge model.
- SaaS stores only a vault reference (e.g., `vault://secret/data/postgres-prod`)
- Agent has local access to the customer's secret store
- On scan execution, agent resolves the vault reference to actual credentials
- Credentials are held in memory only for the duration of the scan
- SaaS never sees or logs actual credentials

### Multi-Tenancy

- Tenant isolation via `tenant_id` column on all tables
- API middleware injects tenant context from authenticated JWT
- Row-level security in Postgres as defense-in-depth
- GCS bundles: shared library (public benchmarks) + tenant-specific buckets (custom bundles)

### Bundle Signing

- Bundles are signed with Ed25519 keys
- Agent verifies signature before execution
- SilkStrand-authored bundles use platform key
- Custom bundles can use tenant-managed keys

## Database Schema (Core Tables)

```sql
-- Tenants
tenants (id, name, created_at)

-- Users (managed via Auth0/Clerk, minimal local record)
users (id, tenant_id, external_id, email, role, created_at)

-- Agents
agents (id, tenant_id, name, status, last_heartbeat, version, created_at)

-- Targets
targets (id, tenant_id, type, identifier, config, environment, created_at)

-- Credentials (MVP only — replaced by vault refs post-MVP)
credentials (id, tenant_id, target_id, type, encrypted_data, created_at)

-- Bundles
bundles (id, tenant_id, name, version, framework, target_type, gcs_path, signature, created_at)

-- Scans
scans (id, tenant_id, agent_id, target_id, bundle_id, status, started_at, completed_at)

-- Scan Results
scan_results (id, scan_id, control_id, title, status, severity, evidence, remediation)
```

## Real-Time Communication (Upstash Redis)

### Channels

- `agent:{agent_id}:directives` — SaaS publishes scan directives for a specific agent
- `agent:{agent_id}:status` — Agent status updates (heartbeat, connected, disconnected)
- `scan:{scan_id}:progress` — Scan progress updates for real-time UI
- `tenant:{tenant_id}:events` — Tenant-wide event stream for dashboard

### Flow

1. Agent connects via WSS to API server
2. API server subscribes to `agent:{agent_id}:directives` on Upstash Redis
3. When a scan is triggered, API publishes to the agent's directive channel
4. The API instance subscribed to that channel receives it and forwards over WSS
5. Results flow back over WSS, API publishes to `scan:{scan_id}:progress`
6. Web frontend subscribes to progress channel via SSE or polling

## Local Development

- `docker-compose.yml` for local Postgres and Redis (standard Redis for dev)
- Agent runs locally, connects to local API
- API serves both REST and WSS endpoints
- Web dev server proxies API requests
- Bundles stored on local filesystem instead of GCS
