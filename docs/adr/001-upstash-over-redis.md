# ADR 001: Upstash Redis over Self-Hosted Redis

## Status

Accepted

## Date

2026-04-11

## Context

SilkStrand needs a pub/sub mechanism to route scan directives from the API server to the correct Cloud Run instance holding an agent's WebSocket connection. Redis pub/sub is the natural fit, but self-hosted Redis (GCP Memorystore) has significant idle cost that conflicts with our cost-minimal architecture.

The alternatives considered:

1. **GCP Memorystore (managed Redis)**: Minimum ~$30/month for the smallest instance. Always-on, even with zero traffic.
2. **Postgres LISTEN/NOTIFY**: Zero additional cost (already have Cloud SQL). But fire-and-forget semantics require careful handling, and mixing pub/sub into the primary database adds coupling.
3. **GCP Cloud Pub/Sub**: Pay-per-message, but adds GCP coupling and more complex API than Redis.
4. **Upstash Redis**: Serverless Redis with pay-per-request pricing. Standard Redis API compatibility. Free tier: 10k commands/day.

## Decision

Use Upstash Redis for real-time pub/sub and lightweight caching.

## Consequences

### Positive

- Zero idle cost — free tier covers MVP and early usage
- Standard Redis API — Go redis clients work unmodified
- No infrastructure to manage — fully managed service
- REST API available — works from serverless environments without persistent connections
- Can migrate to self-hosted Redis later if needed (standard API)

### Negative

- External dependency outside GCP — adds a vendor
- Latency slightly higher than in-VPC Redis (~5-15ms vs ~1ms)
- Rate limits on free tier (10k commands/day) may require paid plan as usage grows

### Mitigations

- The latency delta is negligible for scan directive delivery (scans take seconds to minutes)
- Upstash paid plans are still significantly cheaper than Memorystore at low-to-moderate volume
- Standard Redis API means we can swap to Memorystore if we outgrow Upstash
