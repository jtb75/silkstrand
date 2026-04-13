# SilkStrand Security Audit v0.0.2
**Date:** 2026-04-13
**Auditor:** Gemini CLI
**Status:** Design Review + Remediation Validation

## 1. Executive Summary

This audit validates the remediation of findings from v0.0.1 and reviews the security implications of the new **Recon Pipeline (ADR 003)** and **Credential Resolver (ADR 004)** designs.

The critical risk of unauthorized code execution via bundles has been successfully mitigated. The architecture is maturing, but the introduction of pluggable credential resolvers and network-wide discovery introduces new trust boundaries that require careful implementation.

---

## 2. Remediation Validation (from v0.0.1)

| Finding | Status | Validation |
| :--- | :--- | :--- |
| **2.1 Bundle Signature Verification** | **RESOLVED** | `agent/internal/cache/cache.go` now implements Ed25519 verification. Verification is mandatory in production (`publicKey != nil`). |
| **2.2 JWT Claim Validation** | **RESOLVED** | `api/internal/middleware/auth.go` now validates HMAC signatures and `exp` claims. (Note: The system shifted from Clerk to in-house HS256 JWTs; validation is now handled by standard HMAC verification). |
| **2.3 WebSocket Origin Check** | **RESOLVED** | `api/internal/websocket/hub.go` uses `AllowedOrigins` (from `ALLOWED_ORIGINS` env). Allows empty origins for non-browser clients (agents). |
| **2.4 Tenant-scoped Results** | **RESOLVED** | `api/internal/store/postgres.go` now JOINs on the `scans` table to enforce `tenant_id` ownership for scan results. |
| **2.5 Encryption Key Fallback** | **RESOLVED** | `api/internal/config/config.go` now fails to start if `CREDENTIAL_ENCRYPTION_KEY` or `INTERNAL_API_KEY` are missing in production. |

---

## 3. New Findings & Design Analysis

### 3.1 Credential Resolver: Trust Boundary Shift (HIGH)
**ADR 004 Context**

**Finding:**
With pluggable resolvers (AWS Secrets Manager, HashiCorp Vault), the agent now requires higher-privileged identity (IAM roles, Vault tokens) to fetch secrets on demand.

**Risk:**
A compromised agent process can now potentially exfiltrate any secret it has permission to resolve, not just the ones for the current scan. If an agent is granted `secretsmanager:GetSecretValue` on a broad prefix, the blast radius of an agent compromise increases significantly.

**Recommendation:**
- **Principle of Least Privilege:** Document and enforce narrow IAM policies (e.g., resource-based policies on secrets limited to specific agent ARNs).
- **Audit Logging:** As noted in ADR 004 "Open Items," the agent MUST log every fetch. The SaaS API should also correlate fetch events with active scan directives.

### 3.2 Recon Pipeline: Network Surface & Rate Limiting (MEDIUM)
**ADR 003 Context**

**Finding:**
The agent will now run active network discovery (`naabu`, `nuclei`).

**Risk:**
Uncontrolled or malicious discovery scans could be used for lateral movement or as a DoS tool against internal infrastructure. A compromised SaaS account could be used to "weaponize" customer agents against their own networks.

**Recommendation:**
- **Agent-Side CIDR Allowlisting:** Allow customers to define "Safe Zones" in the agent's local configuration that the SaaS cannot override.
- **Global Rate Limiting:** Implement hard-coded packets-per-second limits in `agent/internal/runner/recon.go`.

### 3.3 Temporary Credential Persistence (LOW)
**Location:** `agent/internal/runner/python.go`

**Finding:**
The runner writes credentials to a temporary file (`credentials.json`) with `0o600` permissions. While it uses `defer os.RemoveAll(tmpDir)`, a crashed process or a system hard-reboot could leave sensitive credentials in plaintext on the agent's disk.

**Risk:**
Short-lived plaintext exposure on disk. If an attacker has local read access to the agent's temp directory, they can capture credentials during the scan window.

**Recommendation:**
- **Memory-only pipes:** Use `os.Pipe` or `stdin` to pass credentials to the Python subprocess instead of a file.
- **Ramdisk:** If files are required, recommend/default the agent's temp directory to a `tmpfs` mount.

### 3.4 Missing JWT Audience/Issuer in Backoffice (LOW)
**Location:** `backoffice/internal/middleware/auth.go`

**Finding:**
The backoffice admin JWT validation only checks the signature and expiration. It does not validate `iss` or `aud`.

**Risk:**
While less critical than the public API (due to smaller user base and distinct secret), it lacks defense-in-depth against token reuse across environments.

**Recommendation:**
Add standard `iss` and `aud` validation to `backoffice/internal/middleware/auth.go`.

---

## 4. Strategic Recommendations

1.  **Ephemeral Agent Identity:** Explore using SPIFFE/SPIRE for agent identity to rotate agent-to-SaaS credentials automatically and provide more granular identity for Vault/AWS integrations.
2.  **Bundle Sandbox:** The move to "PD tools" (Nuclei, etc.) in the recon pipeline increases the binary dependency surface. Consider running these in a restricted container or a low-privilege user sandbox on the host.
3.  **Result Masking:** Implement server-side masking of sensitive fields in `evidence` JSONB to prevent PII or credentials found by Nuclei from being stored permanently in the SaaS DB.

## 5. Conclusion

SilkStrand has made excellent progress in securing its core pipeline. The transition to a Recon + Compliance platform is architecturally sound but requires a shift in security focus toward **Agent-side policy enforcement** and **Credential lifecycle management**.
