#!/usr/bin/env bash
set -euo pipefail

# Generates Bicep types.json files for the default contrib resource type namespaces listed in defaults.yaml.
# Each entry in defaults.yaml uses <namespace>/<typeName> format (e.g. Radius.Compute/containers).
# Per-type manifest files live under the manifest dir as individual YAML files.
#
# Usage: generate-bicep-types-contrib.sh <defaults_yaml> <manifest_dir> <output_base> <api_version> <emitter_dir>
#
# Arguments:
#   defaults_yaml  - Path to defaults.yaml listing default resource type registrations.
#   manifest_dir   - Directory containing per-type manifest YAML files.
#   output_base    - Base output directory for generated Bicep types.
#   api_version    - API version string to use for the output directory name.
#   emitter_dir    - Directory of the built TypeSpec Bicep emitter package (provides
#                    dist/src/cmd/generate-contrib-docs.js for reference-doc generation).

DEFAULTS_YAML="${1:?defaults_yaml argument is required}"
BICEP_TYPES_CONTRIB_MANIFEST_DIR="${2:?manifest_dir argument is required}"
BICEP_TYPES_OUTPUT_BASE="${3:?output_base argument is required}"
BICEP_TYPES_CONTRIB_API_VERSION="${4:?api_version argument is required}"
BICEP_TYPES_EMITTER_DIR="${5:?emitter_dir argument is required}"

GENERATE_DOCS_JS="${BICEP_TYPES_EMITTER_DIR}/dist/src/cmd/generate-contrib-docs.js"
if [[ ! -f "${GENERATE_DOCS_JS}" ]]; then
    echo "ERROR: Reference-doc generator not found: ${GENERATE_DOCS_JS}"
    echo "Build the emitter first (make generate-bicep-types-emitter)."
    exit 1
fi

NAMESPACES=$(yq '.defaultRegistration[]' "$DEFAULTS_YAML" | sed 's|/.*||' | sort -u)
for ns in $NAMESPACES; do
    ns_lower=$(echo "$ns" | tr '[:upper:]' '[:lower:]')
    out_dir="$BICEP_TYPES_OUTPUT_BASE/$ns_lower/$BICEP_TYPES_CONTRIB_API_VERSION"
    manifest_args=""
    for entry in $(yq '.defaultRegistration[]' "$DEFAULTS_YAML"); do
        # Literal namespace prefix match; namespaces contain '.', which grep would treat as a wildcard.
        [ "${entry%%/*}" = "$ns" ] || continue
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

    # Resource-level descriptions are authored in the manifests but cannot be carried in
    # types.json: the bicep-types schema has no description field on ResourceType or
    # ObjectType, only on individual properties. Side-channel them to the doc generator as
    # a { "<Namespace>/<typeName>": "<description>" } map, mirroring how the TypeSpec
    # emitter reads @doc from the compiler rather than from types.json.
    #
    # Only the first document of each manifest is read, because manifest-to-bicep parses
    # with yaml.Unmarshal and silently ignores any later documents. Reading them here too
    # would describe types the converter never emitted.
    descriptions_json="$out_dir/descriptions.json"
    # shellcheck disable=SC2016 # $ns is a yq variable, so the expression must not be expanded by the shell.
    yq -o=json eval-all \
        '[select(document_index == 0) | .namespace as $ns | .types | to_entries[] | select(.value.description != null) | {"key": ($ns + "/" + .key), "value": .value.description}] | from_entries' \
        $manifest_args > "$descriptions_json"

    node "${GENERATE_DOCS_JS}" \
        --types-json "$out_dir/types.json" \
        --out-dir "$out_dir/docs" \
        --descriptions "$descriptions_json"

    rm -f "$descriptions_json"
done
