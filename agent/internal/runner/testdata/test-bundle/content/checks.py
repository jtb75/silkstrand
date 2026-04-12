#!/usr/bin/env python3
"""Minimal test bundle that outputs valid results JSON."""

import json
import os
from datetime import datetime, timezone

# Read target config (verifies the env var is set correctly)
config_path = os.environ.get("SILKSTRAND_TARGET_CONFIG", "")
if config_path:
    with open(config_path) as f:
        target_config = json.load(f)
else:
    target_config = {}

now = datetime.now(timezone.utc).isoformat()

results = {
    "schema_version": "1",
    "bundle": {
        "name": "test-bundle",
        "version": "1.0.0"
    },
    "target": {
        "type": target_config.get("type", "database"),
        "identifier": target_config.get("identifier", "localhost:5432")
    },
    "started_at": now,
    "completed_at": now,
    "status": "completed",
    "summary": {
        "total": 1,
        "pass": 1,
        "fail": 0,
        "error": 0,
        "not_applicable": 0
    },
    "controls": [
        {
            "id": "1.1",
            "title": "Test Control",
            "description": "A test control for validation",
            "status": "PASS",
            "severity": "LOW",
            "evidence": {
                "current_value": "expected",
                "expected_value": "expected",
                "source": "test"
            },
            "remediation": "No action needed."
        }
    ]
}

print(json.dumps(results))
