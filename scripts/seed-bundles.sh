#!/usr/bin/env bash
# Seed the three CIS bundles into the DC database by parsing bundle.yaml
# and control.yaml files from the bundles/ directory and inserting directly
# via SQL. Idempotent — safe to run multiple times.
#
# Usage: scripts/seed-bundles.sh
# Requires: psql, yq (or falls back to grep/sed parsing)
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-15432}"
DB_USER="${DB_USER:-silkstrand}"
DB_NAME="${DB_NAME:-silkstrand}"
DB_PASSWORD="${PGPASSWORD:-localdev}"

export PGPASSWORD="$DB_PASSWORD"

psql_cmd() {
  psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -A -c "$1"
}

# Parse a YAML value (simple single-line fields only).
yaml_val() {
  local file="$1" key="$2"
  sed -n "s/^${key}:[[:space:]]*//p" "$file" | sed 's/^"\(.*\)"$/\1/' | tr -d "'" | head -1
}

# Parse the controls list from bundle.yaml.
yaml_controls() {
  local file="$1"
  sed -n '/^controls:/,/^[^ ]/p' "$file" | grep '^  - ' | sed 's/^[[:space:]]*-[[:space:]]*//'
}

for bundle_dir in "$ROOT_DIR"/bundles/cis-*/; do
  manifest="$bundle_dir/bundle.yaml"
  [ -f "$manifest" ] || continue

  bundle_id=$(yaml_val "$manifest" "id")
  bundle_name=$(yaml_val "$manifest" "name")
  bundle_version=$(yaml_val "$manifest" "version")
  bundle_framework=$(yaml_val "$manifest" "framework")
  bundle_engine=$(yaml_val "$manifest" "engine")

  [ -z "$bundle_id" ] && { echo "SKIP: no id in $manifest"; continue; }

  echo "Seeding bundle: $bundle_name ($bundle_id)"

  # Count controls.
  controls=$(yaml_controls "$manifest")
  control_count=$(echo "$controls" | wc -l | tr -d '[:space:]')

  # Upsert bundle row. Use bundle_id as the framework name for target_type
  # (legacy compat — engine doubles as target_type).
  psql_cmd "
    INSERT INTO bundles (id, name, version, framework, target_type, engine, control_count)
    VALUES ('$bundle_id', '$bundle_name', '$bundle_version', '$bundle_framework', '$bundle_engine', '$bundle_engine', $control_count)
    ON CONFLICT (id) DO UPDATE SET
      name = EXCLUDED.name,
      version = EXCLUDED.version,
      framework = EXCLUDED.framework,
      target_type = EXCLUDED.target_type,
      engine = EXCLUDED.engine,
      control_count = EXCLUDED.control_count;
  " > /dev/null

  # Look up the actual UUID (the id column might differ if it was inserted
  # as a text that gets cast). For CIS bundles the id IS the bundle_id.
  actual_id=$(psql_cmd "SELECT id FROM bundles WHERE id = '$bundle_id'" | head -1)
  if [ -z "$actual_id" ]; then
    echo "  ERROR: bundle row not found after upsert"
    continue
  fi

  # Delete existing controls for this bundle before re-inserting.
  psql_cmd "DELETE FROM bundle_controls WHERE bundle_id = '$actual_id'" > /dev/null

  # Resolve control yamls from the content/controls/ directory.
  controls_dir="$bundle_dir/content/controls"
  inserted=0

  echo "$controls" | while IFS= read -r ctrl_id; do
    [ -z "$ctrl_id" ] && continue

    # Find the matching control yaml. The legacy format uses section-based
    # filenames (e.g., 6.8-tls-enabled.yaml) rather than control IDs.
    # We search for a yaml whose filename contains a substring match.
    ctrl_yaml=""
    if [ -d "$controls_dir" ]; then
      # Try to match by filename substring after the section number.
      for f in "$controls_dir"/*.yaml; do
        [ -f "$f" ] || continue
        ctrl_yaml="$f"
        # Check if this yaml's id field maps to the control
        yaml_id=$(yaml_val "$f" "id")
        # We'll just process all of them below — the manifest controls list
        # tells us which controls belong to this bundle.
        break
      done
    fi

    # For the seed, we iterate the manifest controls list and find the
    # matching yaml by scanning all yamls for each control ID. This is
    # O(n*m) but the sets are small.
    found_yaml=""
    if [ -d "$controls_dir" ]; then
      # Build a mapping: the bundle.yaml control ID (e.g., pg-tls-enabled)
      # maps to a content/controls yaml. The yaml's own id field is a section
      # number (e.g., "6.8"). We need to find which yaml corresponds to which
      # manifest control ID. The mapping is positional: bundle.yaml controls
      # are in the same order as the alphabetical sort of the yaml filenames.
      # Actually, that's not reliable. Let's just match by looking at the
      # control ID suffix in the filename.
      suffix=$(echo "$ctrl_id" | sed 's/^[a-z]*-//')  # e.g., pg-tls-enabled -> tls-enabled
      for f in "$controls_dir"/*.yaml; do
        [ -f "$f" ] || continue
        fname=$(basename "$f" .yaml)
        # Check if the filename contains the suffix (after stripping section prefix).
        fname_suffix=$(echo "$fname" | sed 's/^[0-9.]*-//')  # e.g., 6.8-tls-enabled -> tls-enabled
        if [ "$fname_suffix" = "$suffix" ]; then
          found_yaml="$f"
          break
        fi
      done
    fi

    # Parse control metadata.
    ctrl_name="$ctrl_id"
    ctrl_severity=""
    ctrl_section=""
    if [ -n "$found_yaml" ]; then
      title=$(yaml_val "$found_yaml" "title")
      severity=$(yaml_val "$found_yaml" "severity")
      section=$(yaml_val "$found_yaml" "section")
      [ -n "$title" ] && ctrl_name="$title"
      [ -n "$severity" ] && ctrl_severity=$(echo "$severity" | tr '[:upper:]' '[:lower:]')
      [ -n "$section" ] && ctrl_section="$section"
    fi

    # Escape single quotes in names for SQL.
    ctrl_name_escaped=$(echo "$ctrl_name" | sed "s/'/''/g")

    # Insert control row.
    sev_val="NULL"
    [ -n "$ctrl_severity" ] && sev_val="'$ctrl_severity'"
    sec_val="NULL"
    [ -n "$ctrl_section" ] && sec_val="'$ctrl_section'"

    psql_cmd "
      INSERT INTO bundle_controls (bundle_id, control_id, name, severity, section, engine, engine_versions, tags)
      VALUES ('$actual_id', '$ctrl_id', '$ctrl_name_escaped', $sev_val, $sec_val, '$bundle_engine', '[]'::jsonb, '[]'::jsonb)
      ON CONFLICT (bundle_id, control_id) DO UPDATE SET
        name = EXCLUDED.name, severity = EXCLUDED.severity, section = EXCLUDED.section;
    " > /dev/null
  done

  echo "  Done ($control_count controls)"
done

echo ""
echo "Bundle seeding complete."
