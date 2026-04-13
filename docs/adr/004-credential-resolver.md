# ADR 004: Credential resolver

**Status:** Proposed
**Date:** 2026-04-13
**Related:** [ADR 003](./003-recon-pipeline.md) (recon pipeline)

---

## Context

Today, compliance targets are paired 1:1 with a row in a separate
`credentials` table (`credentials.target_id` has a unique index) holding
an AES-GCM encrypted username/password blob. This works for the
manual-target, known-tenant-count world we built for compliance v1, but
breaks down as we add:

- **Recon-driven promote-to-compliance.** Auto-creating targets from
  discovered assets requires auto-resolving credentials. Customers
  with 50 discovered Postgres instances won't fill in 50 passwords.
- **Enterprise secret management.** Customers want credentials to live
  in the secret store they already run — HashiCorp Vault, AWS Secrets
  Manager, Azure Key Vault, GCP Secret Manager, CyberArk. Not in our
  database, even encrypted.
- **Credential rotation.** When a vault rotates, we want the next scan
  to pick up the new credential automatically, not fail until the
  admin re-enters the password.
- **Dynamic credentials.** The strongest security story (Vault DB
  secrets engine) provisions a fresh ephemeral credential per scan.
  Static storage precludes this.

## Decision

Targets reference a `credential_source_id` instead of storing
credentials directly. Each credential source is a pluggable resolver
that fetches credentials at scan time from an external store (or, for
the `static` type, from our existing encrypted storage).

```sql
credential_sources (
  id UUID PK,
  tenant_id UUID NOT NULL,
  type TEXT NOT NULL,     -- 'static' | 'aws_secrets_manager' | ...
  config JSONB NOT NULL,  -- shape varies per type
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- new FK on targets, backfilled from the existing credentials table:
targets.credential_source_id UUID REFERENCES credential_sources(id);
```

Credentials are **never persisted** for sources other than `static`.
The agent fetches fresh credentials at scan start, uses them, and
discards at scan end.

## Source types

| Type | Config shape | Agent auth | Notes |
|---|---|---|---|
| `static` | `{type, encrypted_data}` | — | Current behaviour, wrapped. `type` preserves today's credential kind (e.g. `"database"`); `encrypted_data` is the AES-256-GCM blob. Fallback for customers without a vault. |
| `aws_secrets_manager` | `{secret_arn}` | IAM role on agent | First non-static source (Phase C1). |
| `gcp_secret_manager` | `{secret_path, version?}` | Service account on agent | |
| `azure_key_vault` | `{vault_url, secret_name}` | Managed Identity / service principal | |
| `vault_kv` | `{addr, path, mount?}` | Vault token with renewal | HashiCorp Vault KV v1/v2. |
| `vault_db_engine` | `{addr, role}` | Vault token | Dynamic credentials — per-scan ephemeral users. Strongest security posture. |
| `cyberark` | `{api_url, account_id}` | CyberArk CCP/Conjur token | |

All types share the same JSON envelope at API level:
`{type, config}`. The agent-side resolver dispatches on `type`.

## Auto-alignment — how discovered assets get a credential source

Four-layer resolver chain, runs in order, stops at first hit:

1. **Cloud-native auto-binding.** For assets discovered via cloud APIs
   (ADR 003 D8), use the provider's own linkage:
   - AWS RDS / Aurora: `MasterUserSecret.SecretArn` from
     `DescribeDBInstances`.
   - Azure SQL: linked Key Vault references.
   - GCP Cloud SQL: IAM bindings.
   
   Deterministic; no customer configuration beyond IAM permissions.
   Authoritative when the cloud API provides it.

2. **Vault inventory + tag matching.** Customer integrates the vault
   (Vault / AWS Secrets Manager / Azure Key Vault / CyberArk) with
   read access. We list secrets and read their metadata (`host=...`,
   `service=...`, `env=...`). Discovery → match by tag.
   
   Documented supported tag keys; customer-configurable mapping.
   Heuristic — works only when customers actually tag their secrets.

3. **Naming-convention rules.** Customer declares a path template
   (e.g. `database/{env}/{technology}/{hostname}`). Resolver constructs
   the path from discovered-asset attributes and attempts a fetch.
   Falls back if the convention misses.

4. **Manual mapping.** Always available. Asset row in the UI has a
   "set credential source" CTA, admin picks from a dropdown of
   integrated providers, supplies path/ARN, done.

The UI shows the resolution path on each asset ("Auto-resolved via AWS
RDS binding" / "Matched Vault tag host=...") so admins can audit and
trust the chain.

## Phases

| Phase | Scope |
|---|---|
| **C0** *(prereq for ADR 003 R0)* | Add `credential_sources` table and `targets.credential_source_id` FK. Add `static` source type wrapping today's encrypted storage. One-time backfill: one `static` credential_sources row per existing `credentials` row, with `targets.credential_source_id` set to match. Legacy `credentials` table is kept read-only through the transition and dropped in a follow-up migration once rollback window closes. **No behaviour change** at the agent / prober / runner interface. Plumbing only. |
| **C1** *(with recon R2)* | AWS Secrets Manager resolver. Cloud-native auto-binding for RDS / Aurora / DocumentDB. Agent IAM role model. Validates the abstraction end-to-end with one provider. |
| **C2** *(continuous)* | `static` remains the universal escape hatch. Manual credential entry UI stays functional for customers without a vault integration. Do not gate the product on integrations. |
| **C3** *(demand-driven)* | HashiCorp Vault — KV engine first, DB secrets engine second. Token renewal strategy. Tag-match and naming-rule resolvers. |
| **C4** *(demand-driven)* | Azure Key Vault, GCP Secret Manager, CyberArk. Each a separate integration; same plugin pattern. |

## Schema sketches

```sql
CREATE TABLE credential_sources (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  config JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_credential_sources_tenant ON credential_sources(tenant_id);

ALTER TABLE targets
  ADD COLUMN credential_source_id UUID REFERENCES credential_sources(id);

-- One follow-up migration after the rollback window closes drops the
-- legacy table:
-- DROP TABLE credentials;
```

Per-type `config` JSON schemas are defined in the resolver plugin for
each type; API validates on insert/update.

## Multi-vault environments

Large enterprises run multiple secret stores simultaneously. Resolver
chain ordering is per-tenant and per-discovered-asset context:

- AWS-discovered assets → try AWS Secrets Manager first.
- On-prem-discovered assets → try Vault / CyberArk first, based on
  per-tenant priority config.
- Manual fallback last.

Configuration shape (v1): a single `tenant.resolver_chain` setting
naming the active integrations in priority order. Revisit if customers
need per-CIDR or per-tag routing.

## Open items

- **Vault token renewal strategy.** Tokens issued for the agent need
  renewal (the Vault lease model). Agent-side vs. API-side renewal,
  TTL selection, handling of renewal failures mid-scan.
- **Ephemeral-credential lifecycle.** Vault DB engine issues a
  credential with a lease; scan must complete within the lease window
  or re-lease. What happens on scan stalls?
- **Credential source sharing across targets.** Multiple targets may
  reference the same secret. Does the UI surface this? Does rotation
  of one source implicitly affect all targets referencing it? (Yes to
  both; worth documenting explicitly in the product.)
- **Secret caching inside a scan.** Agent fetches credential at scan
  start; if the scan is long and the secret rotates mid-scan, fetched
  credential becomes stale. Probably acceptable — a single scan is a
  short atomic operation relative to rotation cadence.
- **Audit logging.** Every credential fetch must be logged (source, target,
  scan_id, outcome). Drives compliance reporting and incident response.
  C0 is the right time to add the structured slog event at the single
  fetch site — retrofitting is more expensive once multiple resolver
  types share the path.
- **Resolver execution boundary.** C0 resolves `static` server-side
  (API decrypts the blob and embeds plaintext in the WSS directive, as
  today). C1+ non-static types (AWS Secrets Manager, Vault, etc.)
  resolve **agent-side** — the API forwards an opaque resolver context
  (`{type, config}`) and the agent performs the fetch using its own
  IAM / token material. C0 must leave this seam clean: even though no
  agent-side resolution is wired yet, the directive payload should
  already carry a typed field that future resolver dispatches can slot
  into without another schema break.

## Consequences

- **Positive**: product scales to enterprise customers who won't store
  credentials in our DB; credentials rotate without admin intervention;
  auto-promote-to-compliance (ADR 003 D2) works end-to-end without
  per-asset credential entry; positions SilkStrand alongside how customers
  already manage secrets rather than in spite of it.
- **Negative**: multiple integrations to build over time, each with its
  own auth model and failure modes; agent gains IAM/token complexity;
  debugging "why did my scan fail to auth?" now spans our system,
  the vault, and the network between them.
- **Neutral**: `static` source remains forever as the escape hatch for
  customers who don't operate a vault. Don't rip it out.
