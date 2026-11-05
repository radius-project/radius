#!/bin/bash

# Generates the versioned bicepconfig.json used by the Bicep container image,
# pointing the Radius and AWS Bicep extensions at the registry tag for the given
# release channel. The Bicep CLI binary itself is installed separately by
# build/scripts/install-bicep.sh, the single source of truth for that.
# Tagged image builds supply the approved plan as the optional third argument.
#
# Usage: ./generate-bicepconfig.sh <release-channel> <output-dir> [release-plan]
# Example: ./generate-bicepconfig.sh edge ./output

set -euo pipefail

REL_CHANNEL=${1:-}
OUTPUT_DIR=${2:-}
RELEASE_PLAN=${3:-}

if [[ -z "${REL_CHANNEL}" ]]; then
    echo "Release channel is required. Please provide it as the first argument." >&2
    exit 1
fi

if [[ -z "${OUTPUT_DIR}" ]]; then
    echo "Output directory is required. Please provide it as the second argument." >&2
    exit 1
fi

if [[ -n "${RELEASE_PLAN}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PLAN_JSON="$(mktemp)"
    trap 'rm -f "${PLAN_JSON}"' EXIT
    yq -o=json '.' "${RELEASE_PLAN}" >"${PLAN_JSON}"
    source_sha="$(git -C "${SCRIPT_DIR}" rev-parse HEAD)"
    if [[ -n "${GITHUB_SHA:-}" && "${source_sha}" != "${GITHUB_SHA}" ]]; then
        echo "Bicep image checkout differs from the release source." >&2
        exit 1
    fi
    config="$(node "${SCRIPT_DIR}/../../.github/scripts/release-bicep-extensions.mjs" \
        config "${PLAN_JSON}" "${source_sha}" "${REL_CHANNEL}")"
    mkdir -p "${OUTPUT_DIR}"
    printf '%s\n' "${config}" >"${OUTPUT_DIR}/bicepconfig.json"
    exit 0
fi

# Create versioned bicepconfig.json
mkdir -p "${OUTPUT_DIR}"
cat <<EOF > "${OUTPUT_DIR}/bicepconfig.json"
{
  "experimentalFeaturesEnabled": {
    "ociEnabled": true
  },
  "extensions": {
    "radius": "br:ghcr.io/radius-project/bicep-types-radius:${REL_CHANNEL}",
    "aws": "br:ghcr.io/radius-project/bicep-types-aws:${REL_CHANNEL}"
  }
}
EOF
