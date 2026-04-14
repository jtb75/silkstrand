# Onboarding UX plan

Ship a tenant admin from "I just got invited" to "I ran a discovery scan and a
compliance scan" without opening a terminal. Then fill in the ongoing-ops
gaps that let them live in the product day to day. Out-of-session horizon:
sketch how AWS cloud discovery (R2 + ADR 004 C1) reshapes the journey so we
don't ship onboarding work we immediately have to undo.

## 1. Target journey

A first-run admin walks through the UI in roughly this order:

1. **Accept invite** → land on dashboard.
2. **Install an agent** in their environment. Get a token, paste a one-liner,
   confirm the agent shows as `connected`.
3. **Configure the agent's scan allowlist** (the customer-owned YAML — today
   this is SCP'd in by hand) and verify the server received the snapshot.
4. **Create a discovery target** — a CIDR or host range the allowlist covers.
5. **Kick a discovery scan**, watch `discovered_assets` populate.
6. **Promote a discovered asset** to a compliance target, set credentials,
   run a compliance scan, read the results.
7. **Automate**: author a rule that suggests or auto-creates compliance
   targets as discovery turns up new hosts, optionally route notifications.

Ongoing, the same admin expects to:
- Upgrade agents from the UI when a new release ships.
- Audit what the system has been doing (rule fires, credential reads,
  notifications).
- Rotate credentials and replace agents without downtime.

## 2. Current state by stage

| # | Stage | API | Tenant UI | Notes |
|---|---|---|---|---|
| 1 | Accept invite / first login | ✅ | ✅ | Done. No SSO / MFA / self-serve. |
| 2a | Generate install token | ✅ | ✅ | Agents page "Install a new agent" panel — already shipped. |
| 2b | Download agent binary | ✅ | ✅ | Agents page surfaces per-platform links. |
| 2c | Agent self-registration | ✅ | ✅ | Agents page shows status / heartbeat. |
| 3a | Customer allowlist YAML on agent | ✅ (agent side) | ❌ | SCP'd by hand; agent pushes snapshot to server but UI doesn't render it. |
| 3b | Server has snapshot for an agent | ✅ (`agent_allowlists`) | ❌ | No viewer/diff surface. |
| 4 | Create discovery target (CIDR / range) | ✅ | ✅ | Targets form has a Kind toggle (Compliance / Discovery) with type picker + identifier + optional ports/rate/httpx/nuclei toggles. |
| 5 | Kick discovery scan | ✅ (`POST /api/v1/scans {scan_type:discovery}`) | ✅ | Scans page has a scan-type picker; discovery uses the global `discovery` bundle seeded in migration 015. |
| 5 | Watch asset ingestion | ✅ | ✅ | Assets page + detail drawer. |
| 6a | Promote discovered → compliance target | ✅ | ✅ | Gated by allowlist status (v0.1.35). |
| 6b | Set / update credentials | ✅ | ✅ | |
| 6c | Run compliance scan | ✅ | ✅ | |
| 6d | View results | ✅ | ✅ | |
| 6e | Edit target (name / identifier / config) | ✅ | ⚠️ | UI only reassigns agent today. Added to deferred list. |
| 7a | Correlation rules | ✅ | ✅ | Match still raw JSON (predicate builder adopted on Asset Sets only). |
| 7b | Notification channels | ✅ | ✅ | Email / PagerDuty rejected by server. Retry worker deferred. |
| 7c | Asset sets | ✅ | ✅ | Visual predicate builder landed v0.1.38. |
| 7d | One-shot scans | ✅ | ✅ | |
| — | Agent upgrade | ✅ (WSS `upgrade` directive) | ✅ | Per-row "Upgrade" button on Agents page — already shipped. |
| — | Audit log surfacing | ⚠️ (slog only) | ❌ | `credential.fetch`, `rule.fired`, delivery rows — nothing queryable. |
| — | Bundle upload | ❌ | ❌ | Bundles seeded via SQL. |
| — | Agent allowlist viewer | ❌ | ❌ | See 3b. |

## 3. Gap severity

**Blocks the "no-terminal first scan" journey (critical path)**
1. Install token button on Agents page.
2. Discovery target creation in the Targets form.
3. Discovery scan launcher.

**Quality-of-life but still UX debt**
4. Target edit flow.
5. Allowlist viewer (server side of it — the snapshot is already stored).
6. Agent upgrade button.
7. Audit log browser (one-page minimum: notification_deliveries + rule fires).

**Ecosystem that unlocks day-two ops**
8. Bundle upload API + UI (ADR-level decision: do we keep seed-only for now?).
9. Self-serve signup + MFA + SSO (not attempting this cycle).

## 4. PR split

Sized to land independently. Each line names the owning pages + the net-new
backend surface, so we can parallelize later if useful.

| PR | Title | Backend | Frontend | Size | Unblocks |
|---|---|---|---|---|---|
| ~~O1~~ | ~~Install token button~~ | — | — | — | **Already shipped** (re-check missed it on plan draft). |
| O2 | Discovery target creation | — | Targets form branch | M | **✅ shipped** |
| O3 | Discovery scan launcher | migration 015 (global discovery bundle row) | Scans page scan-type picker | S | **✅ shipped** |
| O4 | Allowlist viewer | `GET /api/v1/agents/{id}/allowlist` | Agents detail panel | S | Closes the "what does my agent accept" question |
| O5 | Target edit flow | none | Targets form reuse | M | Rename / re-identify / config edit |
| ~~O6~~ | ~~Agent upgrade trigger~~ | — | — | — | **Already shipped** (per-row button exists). |
| O7 | Audit surface v1 | `GET /api/v1/audit-log` (new, read-only) | new Audit page | M–L | Day-two transparency |
| O8 | Correlation rule predicate builder adoption | none | CorrelationRules form split + new ActionListEditor | M | Nit polish; defer unless asked |

Suggested execution order: **O4 → O5 → O7** (O1, O2, O3, O6 done).

Each PR cycles through stage → tag → prod the same way we've been doing,
averaging ~1 tag per PR.

## 5. Ongoing ops decisions

**Agent upgrade**: O6 covers the tenant-initiated case. Not in scope:
tenant-wide "upgrade all agents" automation, scheduled maintenance windows,
per-tenant pinning. Revisit after we've seen how often admins actually use
per-agent upgrade.

**Audit log (O7)**: minimum viable is a table of
`notification_deliveries` (already structured) + a `rule_fires` table we'd
need to add (today rule fires log as slog only). Proposal: add a
lightweight `audit_events` table that captures `rule.fired`,
`credential.fetch`, `notification.sent`, `agent.upgraded`, keyed by
`tenant_id` + `occurred_at` for indexed scans. Keep write path behind a
boolean flag so it's easy to gate/rollback.

**Credential rotation**: out of scope for onboarding-UX; handled by ADR 004
C1+ resolvers, which let customers rotate at the source system.

**Agent replacement**: current flow is delete-and-reinstall with a fresh
install token. That's acceptable. A future "rotate this agent's key"
button is cheap (O6-sized) if the appetite shows up.

**Bundle upload**: explicitly deferred. Bundle authorship is a separate
tool chain and the R1.x target list doesn't require tenant-authored
bundles. Revisit when we have a second bundle shipped.

## 6. How AWS cloud discovery (R2 + ADR 004 C1) reshapes the journey

R2 introduces `target_type: aws_account`, which changes stages 3–5
fundamentally:

- **Stage 3 (allowlist)**: the customer allowlist YAML gates *network* scans
  (naabu/httpx/nuclei). Cloud discovery against the AWS API is governed by
  **IAM role assumption**, not the YAML. The onboarding flow needs to
  branch: network-range targets → configure allowlist YAML; aws_account
  targets → configure a trust relationship so the agent's IAM role can
  `sts:AssumeRole` into the customer's read-only inventory role.
- **Stage 4 (create target)**: a new target-type branch in the Targets
  form: account ID, trust policy assumptions, optional regions. Closer to
  "configure an integration" than "paste a CIDR".
- **Stage 5 (kick discovery)**: same API (`scan_type: discovery`), but the
  agent's discovery runner dispatches to the AWS SDK instead of nmap
  tools. The scan launcher (O3) is unchanged in shape; it just gains a
  `kind: aws_account | network_range` variant in the picker.
- **Stage 6 (promote + credentials)**: cloud-native credential
  auto-binding (ADR 004 §C1) means promoting an RDS instance already has
  its `MasterUserSecret` ARN attached — the "set credential" modal
  collapses to "confirm which Secrets Manager entry to resolve at scan
  time". The existing `UpsertStaticCredentialSource` becomes one branch;
  new `UpsertSecretsManagerCredentialSource` slots alongside it.

**What this means for current PRs**

- O2 (discovery target creation) should route the form through a
  `TargetTypePicker` abstraction so adding `aws_account` later is a new
  branch, not a rewrite. Don't hardcode "CIDR or range" as the only
  discovery type.
- O3 (discovery scan launcher) should treat the target itself as the
  source of truth for how the scan dispatches; UI doesn't need to care
  network vs. cloud.
- O5 (target edit flow) should reuse the same TargetTypePicker so editing
  a cloud target later is natural.
- O7 (audit log) should include `cloud.assume_role` in its event taxonomy
  from day one so C1 doesn't force a retrofit.

**What this means for future PRs (not this plan)**

R2 itself will need, at minimum: AWS credential-source type in `credential_sources`,
agent-side AWS SDK runner, cloud inventory schema extensions to
`discovered_assets` metadata, IAM trust-setup flow in the UI. That's a
distinct plan (likely `docs/plans/r2-aws-discovery.md`).

## 7. Out of scope for this plan

- Self-serve signup, SSO (SAML/OIDC), MFA.
- Multi-region data residency UX (backoffice already handles DC assignment;
  tenant admin doesn't need a UI knob yet).
- Tenant-level quotas / billing.
- Agent observability dashboards beyond "connected / last heartbeat".
- Bundle authorship tooling.

## 8. Open questions

- Does O7 (audit) warrant its own ADR before we add a new table? I lean
  yes — partition strategy (monthly like `notification_deliveries`?) and
  write-path volume are design decisions worth capturing.
- Is install-token generation a super-admin action or can any tenant admin
  self-serve? Current API has no role gate on `POST /api/v1/agents/install-tokens`.
- Do we want per-agent allowlist editing in the UI (a step beyond O4's
  read-only viewer)? Reaching into the agent host from SaaS re-opens the
  "customer owns their scan policy" principle — probably a hard no, but
  worth writing down.
