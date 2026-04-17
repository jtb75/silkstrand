# SilkStrand Security Audit v0.0.3
**Date:** 2026-04-17
**Auditor:** Gemini CLI
**Status:** In-Depth Code Review

## 1. Executive Summary

This audit evaluates the security posture of the SilkStrand platform following the implementation of the **Asset-First Data Model (ADR 006)** and the **Agent Log Streaming (ADR 008)** features. 

The platform has matured significantly, with robust rate limiting for discovery scans and improved credential handling in the agent runner. However, a high-impact vulnerability was identified in the **SaaS Ingest Pipeline**, where a lack of cross-verification between agents and scan IDs could allow tenant data pollution. Additionally, the **Bundle Verification** mechanism remains incomplete, providing only partial protection against bundle tampering.

### Risk Distribution
- **Critical:** 0
- **High:** 1
- **Medium:** 2
- **Low:** 2

---

## 2. Findings & Recommendations

### 2.1 Spoofable Scan Results (HIGH)
**Location:** `api/cmd/silkstrand-api/main.go` (`handleScanResults`, `handleAssetDiscovered`)

**Finding:**
The API receives `scan_results` and `asset_discovered` messages from agents over a WebSocket connection. While the connection itself is authenticated, the handlers for these messages load the `scan` record by `scan_id` but do not verify that the `scan.AgentID` matches the `agentID` of the active connection.

**Risk:**
An attacker who compromises one agent (or obtains a valid `scan_id` via other means) can report malicious discovery or compliance results for any active `scan_id` in the system, potentially polluting the data of other tenants or masking security vulnerabilities.

**Recommendation:**
Update `handleScanResults` and `handleAssetDiscovered` to verify ownership:
```go
scan, err := s.GetScanByID(ctx, wrapper.ScanID)
if err != nil || scan == nil {
    return
}
if scan.AgentID == nil || *scan.AgentID != agentID {
    slog.Warn("unauthorized scan result report", "agent_id", agentID, "scan_id", scan.ID)
    return
}
```

---

### 2.2 Incomplete Bundle Verification (MEDIUM)
**Location:** `agent/internal/cache/cache.go`

**Finding:**
The agent implements Ed25519 signature verification, but it only verifies the `manifest.yaml` file. The actual execution logic (e.g., `python.go`) runs `checks.py` (or the entrypoint defined in the manifest) directly from the filesystem.

**Risk:**
Because the `manifest.yaml` does not contain a hash of the `checks.py` file or other bundle contents, an attacker with write access to the agent's local bundle cache can modify the execution scripts without invalidating the manifest's signature.

**Recommendation:**
- **Option A:** Include a `files` map in `manifest.yaml` containing SHA-256 hashes of all files in the bundle, and verify these hashes before execution.
- **Option B:** Sign a `SHA256SUMS` file that covers the entire bundle content.

---

### 2.3 Credential Exfiltration via Redis (MEDIUM)
**Location:** `api/internal/handler/agent.go` (`forwardDirective`)

**Finding:**
`AgentHandler.forwardDirective` fetches credentials for a `TargetID` and sends them to an agent. The `TargetID` is taken directly from the `pubsub.Directive` received via Redis.

**Risk:**
This represents a defense-in-depth weakness. If an attacker gains the ability to publish messages to the internal Redis bus, they can craft a `Directive` with an arbitrary `TargetID` and send it to an `agentID` they control. The API will then fetch and send the decrypted credentials for that target to the attacker's agent.

**Recommendation:**
Verify that the `target.TenantID` matches the `agent.TenantID` within the `forwardDirective` logic, rather than trusting the `TenantID` provided in the Redis payload.

---

### 2.4 Optional Upgrade Verification (LOW)
**Location:** `agent/internal/updater/updater.go`

**Finding:**
The `Apply` function for agent upgrades skips SHA-256 verification if the `expectedSHA256` parameter is empty. The `AgentsHandler.Upgrade` API does not require this parameter.

**Risk:**
While the `baseURL` is server-controlled, skipping checksum verification is a deviation from security best practices and increases the risk of MITM or registry compromise leading to malicious code execution.

**Recommendation:**
Make SHA-256 verification mandatory in `updater.go` for all production builds.

---

### 2.5 Transitional JWT Claim Validation (LOW)
**Location:** `api/internal/middleware/auth.go`, `backoffice/internal/middleware/auth.go`

**Finding:**
The `iss` and `aud` claims are validated only if they are present in the JWT.

**Risk:**
While this facilitates a rolling update of the token format, it leaves a window where "legacy" tokens (which lack these claims) can be used. This lacks defense-in-depth against token replay across different service boundaries (e.g., using a backoffice admin token against a tenant API).

**Recommendation:**
Set a hard deadline to switch from "transitional" to "mandatory" validation for these claims.

---

## 3. Remediation Validation (from v0.0.2)

| Finding | Status | Validation |
| :--- | :--- | :--- |
| **3.2 Recon Pipeline: Rate Limiting** | **RESOLVED** | `agent/internal/runner/recon/ratelimit.go` implements a hardcoded `MaxGlobalPPS` of 1000. |
| **3.2 Recon Pipeline: Allowlisting** | **RESOLVED** | `agent/internal/runner/recon/allowlist.go` implements a robust fail-closed local policy. |
| **3.3 Temporary Credential Persistence** | **RESOLVED** | `agent/internal/runner/python.go` now uses `os.Pipe` (`/dev/fd/3`) on Unix to pass credentials. |

---

## 4. Strategic Recommendations

1.  **Strict Bundle Integrity:** Implement a "Manifest of Manifests" or include file hashes in the signed `manifest.yaml` to ensure the entire bundle is immutable once signed.
2.  **Internal API Hardening:** Ensure all `internal/v1` routes used by the backoffice are strictly bound to the internal network interface to reduce the exposure of the `X-API-Key`.
3.  **Audit Log Enrichment:** Correlate `credential.fetch` logs in the API with the originating `user_id` or `scan_definition_id` to provide a complete audit trail of secret access.

## 5. Conclusion

SilkStrand continues to build a strong security foundation. The resolution of credential persistence on disk is a significant improvement. Addressing the **Scan ID verification** in the ingest pipeline should be the immediate priority to ensure the integrity of tenant data as the platform scales.
