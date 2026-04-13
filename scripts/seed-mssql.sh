#!/usr/bin/env bash
# Seed the local SQL Server 2022 dev container with a read-only scan user
# that has the system-view permissions CIS MSSQL 2022 audit queries need.
#
# Idempotent — safe to re-run.

set -eu

CONTAINER=${CONTAINER:-silkstrand-mssql-1}
SA_PASSWORD=${MSSQL_SA_PASSWORD:-SilkstrandLocal1!}
SCAN_USER=${SCAN_USER:-silkstrand_scanner}
SCAN_PASSWORD=${SCAN_PASSWORD:-ScannerLocal1!}

# Pick whichever container the user actually has.
if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  # Fallback: find any container built from the mssql image in this compose project.
  CANDIDATE=$(docker ps --filter ancestor=mcr.microsoft.com/mssql/server:2022-latest \
                        --format '{{.Names}}' | head -n1)
  if [ -n "$CANDIDATE" ]; then
    CONTAINER=$CANDIDATE
  else
    echo "error: no running mssql container found (expected '$CONTAINER')" >&2
    echo "hint:  docker compose up -d mssql" >&2
    exit 1
  fi
fi

echo "==> seeding $CONTAINER"

# sqlcmd lives at /opt/mssql-tools18/bin in the 2022 image; -C trusts the self-signed cert.
exec_sql() {
  docker exec -i "$CONTAINER" /opt/mssql-tools18/bin/sqlcmd \
    -S localhost -U sa -P "$SA_PASSWORD" -C -No -Q "$1"
}

# Create the scan login + user at server scope. The scanner reads config
# views, not actual data, so VIEW SERVER STATE + VIEW ANY DEFINITION is
# sufficient for the automated controls in sections 2-5 of the benchmark.
exec_sql "
IF NOT EXISTS (SELECT 1 FROM sys.server_principals WHERE name = '$SCAN_USER')
    CREATE LOGIN [$SCAN_USER] WITH PASSWORD = '$SCAN_PASSWORD', CHECK_POLICY = OFF;
GRANT VIEW SERVER STATE TO [$SCAN_USER];
GRANT VIEW ANY DEFINITION TO [$SCAN_USER];
GRANT VIEW ANY DATABASE TO [$SCAN_USER];
"

echo "==> scan user ready:"
echo "     host=localhost  port=11433  database=master"
echo "     username=$SCAN_USER"
echo "     password=$SCAN_PASSWORD"
