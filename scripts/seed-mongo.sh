#!/usr/bin/env bash
# Seed the local MongoDB container with a read-only scan user for
# CIS MongoDB benchmark evaluation. Idempotent — safe to re-run.

set -eu

CONTAINER=${CONTAINER:-silkstrand-mongodb-1}
ROOT_USER=${ROOT_USER:-silkstrand}
ROOT_PASSWORD=${ROOT_PASSWORD:-MongoLocal1!}
SCAN_USER=${SCAN_USER:-silkstrand_scanner}
SCAN_PASSWORD=${SCAN_PASSWORD:-ScannerLocal1!}

if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER"; then
  CANDIDATE=$(docker ps --filter ancestor=mongo:8 --format '{{.Names}}' | head -n1)
  if [ -n "$CANDIDATE" ]; then
    CONTAINER=$CANDIDATE
  else
    echo "error: no running mongo container found (expected '$CONTAINER')" >&2
    echo "hint:  docker compose up -d mongodb" >&2
    exit 1
  fi
fi

echo "==> seeding $CONTAINER"

docker exec -i "$CONTAINER" mongosh --quiet \
  --username "$ROOT_USER" --password "$ROOT_PASSWORD" \
  --authenticationDatabase admin <<EOF
use admin;
if (!db.getUser("$SCAN_USER")) {
  db.createUser({
    user: "$SCAN_USER",
    pwd: "$SCAN_PASSWORD",
    roles: [ { role: "clusterMonitor", db: "admin" },
             { role: "readAnyDatabase", db: "admin" },
             { role: "userAdminAnyDatabase", db: "admin" } ]
  });
  print("created $SCAN_USER");
} else {
  print("$SCAN_USER already exists");
}
EOF

echo "==> scan user ready:"
echo "     host=localhost  port=27018"
echo "     username=$SCAN_USER"
echo "     password=$SCAN_PASSWORD"
echo "     auth_source=admin"
