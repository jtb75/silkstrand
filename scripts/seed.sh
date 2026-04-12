#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Seeding DC database..."
PGPASSWORD=localdev psql -h localhost -p 15432 -U silkstrand -d silkstrand -f "$SCRIPT_DIR/seed-dc.sql"

echo "Seeding backoffice database..."
PGPASSWORD=localdev psql -h localhost -p 15433 -U silkstrand -d silkstrand_backoffice -f "$SCRIPT_DIR/seed-backoffice.sql"

echo ""
echo "Done. Test credentials:"
echo "  Agent ID:  00000000-0000-0000-0000-000000000010"
echo "  Agent Key: test-agent-key"
echo "  Admin:     admin@silkstrand.io / admin123"
echo "  JWT:       python3 scripts/gen-jwt.py"
