#!/usr/bin/env python3
"""Test control that always passes."""
import json

result = {
    "control_id": "test-control",
    "status": "pass",
    "severity": "low",
    "title": "Test control",
    "evidence": {"detail": "always passes"},
    "remediation": "none needed"
}
print(json.dumps(result))
