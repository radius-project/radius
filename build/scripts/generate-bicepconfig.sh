#!/bin/bash

# Generates the versioned bicepconfig.json used by the Bicep container image,
# pointing the Radius and AWS Bicep extensions at the registry tag for the given
# release channel. The Bicep CLI binary itself is installed separately by
# build/scripts/install-bicep.sh, the single source of truth for that.
#
# Usage: ./generate-bicepconfig.sh <release-channel> <output-dir>
# Example: ./generate-bicepconfig.sh edge ./output

set -euo pipefail

REL_CHANNEL=${1:-}
OUTPUT_DIR=${2:-}

if [[ -z "${REL_CHANNEL}" ]]; then
    echo "Release channel is required. Please provide it as the first argument." >&2
    exit 1
fi

if [[ -z "${OUTPUT_DIR}" ]]; then
    echo "Output directory is required. Please provide it as the second argument." >&2
    exit 1
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
