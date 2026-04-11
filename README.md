# SilkStrand

Network-based authenticated CIS compliance scanner. SaaS-delivered, private-environment capable.

## What is SilkStrand?

SilkStrand is a cloud-native compliance scanning platform that assesses CIS benchmarks against databases, operating systems, and infrastructure — even in private environments that aren't directly accessible from the internet.

A lightweight agent deployed in the customer's environment establishes an outbound-only encrypted tunnel. The SaaS platform orchestrates scans, while all sensitive data collection and assessment happens locally. Only structured compliance results leave the customer network.

## Key Capabilities

- **Private environment scanning** via outbound-only agent (no inbound firewall rules)
- **Authenticated scanning** of databases, OS, and infrastructure
- **Polyglot compliance bundles** — Python, OVAL, Rego, Perl
- **Credential zero-knowledge** — SaaS never sees target credentials (post-MVP)
- **Multi-tenant SaaS** with tenant data isolation

## Architecture

```
SilkStrand SaaS (GCP Cloud Run)
        │
    WSS over 443
    (outbound only)
        │
SilkStrand Agent (customer env)
        │
    Scan Targets
  (DB, OS, Cloud)
```

See [docs/architecture.md](docs/architecture.md) for the full system design.

## Tech Stack

- **Agent**: Go (single binary)
- **API**: Go (Cloud Run)
- **Frontend**: React + TypeScript
- **Database**: PostgreSQL (Cloud SQL)
- **Real-time**: Upstash Redis
- **Infrastructure**: Terraform on GCP

## Project Structure

```
agent/          # Edge agent (Go)
api/            # API server (Go)
web/            # Frontend (React + TypeScript)
terraform/      # GCP infrastructure
bundles/        # Compliance bundle specs & examples
docs/           # Architecture, user stories, ADRs
```

## Development

Coming soon. See [docs/architecture.md](docs/architecture.md) for local development setup plans.

## License

Proprietary. All rights reserved.
