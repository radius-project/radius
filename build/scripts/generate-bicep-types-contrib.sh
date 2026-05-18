#!/usr/bin/env bash
set -euo pipefail

# Generates Bicep types.json files for the default contrib resource type namespaces listed in defaults.yaml.
# Each entry in defaults.yaml uses <namespace>/<typeName> format (e.g. Radius.Compute/containers).
# Per-type manifest files live under the manifest dir as individual YAML files.
#
# Usage: generate-bicep-types-contrib.sh <defaults_yaml> <manifest_dir> <output_base> <api_version>
#
# Arguments:
#   defaults_yaml  - Path to defaults.yaml listing default resource type registrations.
#   manifest_dir   - Directory containing per-type manifest YAML files.
#   output_base    - Base output directory for generated Bicep types.
#   api_version    - API version string to use for the output directory name.

DEFAULTS_YAML="${1:?defaults_yaml argument is required}"
BICEP_TYPES_CONTRIB_MANIFEST_DIR="${2:?manifest_dir argument is required}"
BICEP_TYPES_OUTPUT_BASE="${3:?output_base argument is required}"
BICEP_TYPES_CONTRIB_API_VERSION="${4:?api_version argument is required}"

NAMESPACES=$(yq '.defaultRegistration[]' "$DEFAULTS_YAML" | sed 's|/.*||' | sort -u)
for ns in $NAMESPACES; do
    ns_lower=$(echo "$ns" | tr '[:upper:]' '[:lower:]')
    out_dir="$BICEP_TYPES_OUTPUT_BASE/$ns_lower/$BICEP_TYPES_CONTRIB_API_VERSION"
    manifest_args=""
    for entry in $(yq '.defaultRegistration[]' "$DEFAULTS_YAML" | grep "^$ns/"); do
        type_name=$(echo "$entry" | cut -d'/' -f2)
        manifest="$BICEP_TYPES_CONTRIB_MANIFEST_DIR/$type_name.yaml"
        if [ ! -f "$manifest" ]; then
            echo "ERROR: Manifest not found: $manifest (from entry '$entry')"
            exit 1
        fi
        manifest_args="$manifest_args $manifest"
    done
    echo "  -> $ns ($manifest_args) -> $out_dir"
    go run ./bicep-tools/cmd/manifest-to-bicep generate $manifest_args "$out_dir"
    mkdir -p "$out_dir/docs"
done
