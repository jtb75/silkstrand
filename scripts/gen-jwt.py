#!/usr/bin/env python3
"""Generate a dev JWT for local testing.

Usage:
    python3 scripts/gen-jwt.py                            # default tenant
    python3 scripts/gen-jwt.py <tenant_id>                # custom tenant
    python3 scripts/gen-jwt.py <tenant_id> <secret>       # custom secret
"""
import hmac, hashlib, base64, json, sys, time


def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()


tenant_id = sys.argv[1] if len(sys.argv) > 1 else "00000000-0000-0000-0000-000000000001"
secret = sys.argv[2] if len(sys.argv) > 2 else "dev-secret-change-in-production"
exp = int(time.time()) + 86400  # 24 hours

header = b64url(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64url(json.dumps(
    {"tenant_id": tenant_id, "user_id": "dev", "exp": exp},
    separators=(",", ":"),
).encode())
sig_input = f"{header}.{payload}"
sig = hmac.new(secret.encode(), sig_input.encode(), hashlib.sha256).digest()
token = f"{sig_input}.{b64url(sig)}"

print(token)
