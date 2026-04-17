#!/usr/bin/env sh
# Usage: scripts/build-bundle.sh <bundle-name> [--sign <private-key-path>]
# Example: scripts/build-bundle.sh cis-postgresql-16
# Example: scripts/build-bundle.sh cis-postgresql-16 --sign keys/bundle.key
#
# Reads bundles/<name>/bundle.yaml, copies referenced controls into a
# staging directory, includes the bundle.yaml, tars + gzips the result
# into dist/<name>-<version>.tar.gz. Optionally signs with the given key.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

BUNDLE_NAME="${1:-}"
SIGN_KEY=""

if [ -z "$BUNDLE_NAME" ]; then
  echo "Usage: scripts/build-bundle.sh <bundle-name> [--sign <private-key-path>]" >&2
  exit 1
fi

shift
while [ $# -gt 0 ]; do
  case "$1" in
    --sign)
      SIGN_KEY="${2:-}"
      if [ -z "$SIGN_KEY" ]; then
        echo "Error: --sign requires a key path argument" >&2
        exit 1
      fi
      shift 2
      ;;
    *)
      echo "Error: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

BUNDLE_DIR="$ROOT_DIR/bundles/$BUNDLE_NAME"
MANIFEST="$BUNDLE_DIR/bundle.yaml"

if [ ! -f "$MANIFEST" ]; then
  echo "Error: manifest not found: $MANIFEST" >&2
  exit 1
fi

# Parse version from bundle.yaml (line: "version: X.Y.Z")
VERSION=$(sed -n 's/^version:[[:space:]]*//p' "$MANIFEST" | tr -d '[:space:]')
if [ -z "$VERSION" ]; then
  echo "Error: could not parse version from $MANIFEST" >&2
  exit 1
fi

# Parse controls list from bundle.yaml (lines starting with "  - <id>")
CONTROLS=""
IN_CONTROLS=0
while IFS= read -r line; do
  case "$line" in
    controls:*)
      IN_CONTROLS=1
      ;;
    "  - "*)
      if [ "$IN_CONTROLS" = 1 ]; then
        ctrl=$(echo "$line" | sed 's/^[[:space:]]*-[[:space:]]*//')
        if [ -z "$CONTROLS" ]; then
          CONTROLS="$ctrl"
        else
          CONTROLS="$CONTROLS
$ctrl"
        fi
      fi
      ;;
    *)
      # Any non-list line after controls: ends the list
      if [ "$IN_CONTROLS" = 1 ]; then
        case "$line" in
          ""|"  "*) ;; # blank or continuation — but we only match "  - " above
          *) IN_CONTROLS=0 ;;
        esac
      fi
      ;;
  esac
done < "$MANIFEST"

if [ -z "$CONTROLS" ]; then
  echo "Error: no controls found in $MANIFEST" >&2
  exit 1
fi

CONTROL_COUNT=$(echo "$CONTROLS" | wc -l | tr -d '[:space:]')

# Set up staging
DIST_DIR="$ROOT_DIR/dist"
STAGING="$DIST_DIR/staging/$BUNDLE_NAME"
rm -rf "$STAGING"
mkdir -p "$STAGING/controls"

# Copy manifest
cp "$MANIFEST" "$STAGING/bundle.yaml"

# Copy each referenced control
echo "$CONTROLS" | while IFS= read -r ctrl_id; do
  CTRL_SRC="$ROOT_DIR/controls/$ctrl_id"
  if [ ! -d "$CTRL_SRC" ]; then
    echo "Error: control directory not found: controls/$ctrl_id" >&2
    exit 1
  fi
  cp -r "$CTRL_SRC" "$STAGING/controls/$ctrl_id"
done

# Copy legacy content/ directory for backwards compat
if [ -d "$BUNDLE_DIR/content" ]; then
  cp -r "$BUNDLE_DIR/content" "$STAGING/content"
fi

# Create tarball
TARBALL="$DIST_DIR/$BUNDLE_NAME-$VERSION.tar.gz"
tar -czf "$TARBALL" -C "$STAGING" .

# Sign if requested
if [ -n "$SIGN_KEY" ]; then
  if [ ! -f "$SIGN_KEY" ]; then
    echo "Error: signing key not found: $SIGN_KEY" >&2
    rm -rf "$STAGING"
    exit 1
  fi
  openssl dgst -sha256 -sign "$SIGN_KEY" -out "$TARBALL.sig" "$TARBALL"
  echo "Signed $TARBALL.sig"
fi

# Clean up staging
rm -rf "$STAGING"

echo "Built $TARBALL ($CONTROL_COUNT controls)"
