# SilkStrand Security Audit v0.0.1
**Date:** 2026-04-11
**Auditor:** Gemini CLI
**Status:** Initial Review

## 1. Executive Summary

A targeted security review of the SilkStrand repository was performed, focusing on authentication, tenant isolation, credential handling, and the agent-to-SaaS communication pipeline. 

The architecture correctly implements fundamental security principles, including multi-tenancy at the database layer and authenticated outbound-only tunnels for agents. However, several high-impact gaps were identified, most notably the **absence of bundle signature verification**, which could lead to unauthorized code execution on customer agents.

### Risk Distribution
- **Critical:** 1
- **High:** 0
- **Medium:** 1
- **Low:** 3

---

## 2. Findings & Recommendations

### 2.1 Missing Bundle Signature Verification (CRITICAL)
**Location:** `agent/internal/cache/cache.go`, `agent/cmd/silkstrand-agent/main.go`

**Finding:**
The documentation states that the agent verifies bundle signatures before execution. However, the current implementation in `cache.go` only checks for the presence of `manifest.yaml`. The agent executes any bundle found in the local cache or provided directory without verifying its origin or integrity.

**Example/Risk:**
If an attacker compromises the GCS bucket `silkstrand-stage-bundles` or gains write access to the agent's local `bundles/` directory, they can replace `checks.py` with a malicious script. The agent will execute this script with its own privileges during the next scan.

**Solution:**
Implement Ed25519 signature verification.
1. Generate a platform-wide public key for SilkStrand-authored bundles.
2. Include a `signature.sig` file in every bundle archive.
3. Update `agent/internal/cache/cache.go` to verify the signature before returning a valid path.

```go
// Example Solution Sketch
func (c *Cache) Verify(bundlePath string, publicKey []byte) error {
    sig, _ := os.ReadFile(filepath.Join(bundlePath, "signature.sig"))
    content, _ := os.ReadFile(filepath.Join(bundlePath, "checks.py"))
    if !ed25519.Verify(publicKey, content, sig) {
        return errors.New("invalid bundle signature")
    }
    return nil
}
```

---

### 2.2 Insufficient JWT Claim Validation (MEDIUM)
**Location:** `api/internal/middleware/auth.go`

**Finding:**
The `validateClerkJWT` function verifies the RSA signature and the `exp` claim but fails to validate the `iss` (Issuer) and `aud` (Audience) claims.

**Example/Risk:**
An attacker could potentially use a valid JWT issued by Clerk for a different application. If that application uses the same metadata structure for `tenant_id`, the API would accept the token as valid for the corresponding SilkStrand tenant.

**Solution:**
Enforce standard claim validation in `validateClerkJWT`:
```go
// In validateClerkJWT
if rawClaims.Iss != "https://your-clerk-instance.clerk.accounts.dev" {
    return nil, fmt.Errorf("invalid issuer")
}
if rawClaims.Aud != "your-client-id" {
    return nil, fmt.Errorf("invalid audience")
}
```

---

### 2.3 Insecure WebSocket Origin Check (LOW)
**Location:** `api/internal/websocket/hub.go`

**Finding:**
The `websocket.Upgrader` is configured with a `CheckOrigin` function that always returns `true`.

**Example/Risk:**
This enables Cross-Site WebSocket Hijacking (CSWSH). If a user is logged into the SilkStrand dashboard, a malicious site could initiate a WebSocket connection to the API on their behalf. While the agent flow uses Bearer tokens (less susceptible to simple CSRF), it remains a significant deviation from security best practices.

**Solution:**
Restrict origins to known frontend domains:
```go
var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        origin := r.Header.Get("Origin")
        return origin == "https://app.silkstrand.io" || origin == "http://localhost:5173"
    },
}
```

---

### 2.4 Missing Tenant Scoping in Results (LOW)
**Location:** `api/internal/store/postgres.go`

**Finding:**
The `GetScanResults` query filters only by `scan_id`. While `scan_id` is a UUID and the preceding `GetScan` call is scoped to the tenant, the result retrieval itself relies on the global uniqueness of the ID without enforcing ownership.

**Example/Risk:**
If a vulnerability allowed an attacker to guess or leak a `scan_id` from another tenant, they might attempt to access results directly if an endpoint skipped the initial `GetScan` scoping check.

**Solution:**
Add `tenant_id` to the `scan_results` table and include it in the query:
```sql
SELECT ... FROM scan_results r
JOIN scans s ON r.scan_id = s.id
WHERE r.scan_id = $1 AND s.tenant_id = $2
```

---

### 2.5 Encryption Key Fallback (LOW)
**Location:** `api/internal/handler/internal.go`

**Finding:**
The `CreateCredential` and `forwardDirective` logic silently falls back to plaintext storage and transmission if `CREDENTIAL_ENCRYPTION_KEY` is not configured.

**Example/Risk:**
A misconfiguration in production (e.g., a missing environment variable) would result in sensitive customer credentials (DB passwords, etc.) being stored in plaintext in the database without any warning or error.

**Solution:**
Enforce encryption in production:
```go
if len(h.credKey) == 0 && os.Getenv("ENV") == "prod" {
    writeError(w, http.StatusInternalServerError, "encryption key required in production")
    return
}
```

---

## 3. Scope of Audit

The audit covered the following directories and files:
- `api/internal/middleware/`: Authentication and tenant isolation.
- `api/internal/crypto/`: AES-256-GCM implementation.
- `api/internal/store/`: PostgreSQL query scoping.
- `agent/internal/tunnel/`: WebSocket communication and heartbeats.
- `agent/internal/runner/`: Python bundle execution.
- `agent/internal/cache/`: Bundle storage and retrieval.
- `backoffice/internal/handler/`: Admin authentication and DC management.

## 4. Conclusion

SilkStrand has a solid security foundation, but the "Thin Agent, Smart Bundles" philosophy requires rigorous integrity checks. **Implementing bundle signing should be the top priority** before any production-grade agents are deployed into customer environments.
