# SilkStrand

Network-based authenticated CIS compliance scanner. SaaS-delivered, private-environment capable.

## What is SilkStrand?

SilkStrand is a cloud-native compliance scanning platform that assesses CIS benchmarks against databases, operating systems, and infrastructure — even in private environments that aren't directly accessible from the internet.

A lightweight agent deployed in the customer's environment establishes an outbound-only encrypted tunnel. The SaaS platform orchestrates scans, while all sensitive data collection and assessment happens locally. Only structured compliance results leave the customer network.

## Key Capabilities

- **Private environment scanning** via outbound-only agent (no inbound firewall rules)
- **Authenticated scanning** of databases, OS, and infrastructure
- **Polyglot compliance bundles** — Python, OVAL, Rego, Perl
- **Data residency** — regional data centers ensure customer data stays in the correct jurisdiction
- **Multi-tenant SaaS** with tenant data isolation and per-agent API key auth
- **Backoffice management** — cross-datacenter visibility and tenant lifecycle management
- **Credential encryption at rest** — AES-256-GCM; post-MVP vault integrations for zero-knowledge

## Architecture

```
Backoffice Manager (control plane)
        │
    HTTPS (/internal/v1/)
        │
┌───────┴────────┐
│  Data Center   │  (per-region deployment)
│  API + DB +    │
│  Redis + GCS   │
└───────┬────────┘
        │
    WSS over 443
    (outbound only)
        │
┌───────┴────────┐
│ SilkStrand     │  (customer environment)
│ Agent          │
│   → Targets    │
│   (DB, OS)     │
└────────────────┘
```

See [docs/architecture.md](docs/architecture.md) for the full system design.

## Tech Stack

- **Agent**: Go (single binary, cross-compiled)
- **DC API**: Go (Cloud Run, per-region)
- **Backoffice API**: Go (Cloud Run, separate deployment)
- **Frontends**: React + TypeScript (Vite)
- **Database**: PostgreSQL 16 (Cloud SQL)
- **Real-time**: Upstash Redis (serverless)
- **Infrastructure**: Terraform on GCP

## Project Structure

```
agent/              # Edge agent (Go binary)
api/                # Data center API server (Go)
backoffice/         # Backoffice manager API + frontend
  web/              # Backoffice React frontend
web/                # Tenant React frontend
bundles/            # Compliance bundles
  cis-postgresql-16/  # CIS PostgreSQL 16 (8 controls)
terraform/          # GCP infrastructure
docs/               # Architecture, user stories, ADRs
```

## Local Development

```bash
make dev              # Start dependencies + DC API
make run-backoffice   # Start backoffice API (port 8081)
cd web && npm run dev              # Tenant UI
cd backoffice/web && npm run dev   # Backoffice UI
```

Requires Docker for local Postgres and Redis. See [CLAUDE.md](CLAUDE.md) for full development setup.

## License

Proprietary. All rights reserved.
