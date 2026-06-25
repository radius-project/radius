#!/bin/bash

# Generates the versioned bicepconfig.json used by the Bicep container image,
# pointing the Radius and AWS Bicep extensions at the registry tag for the given
# release channel. The Bicep CLI binary itself is installed separately by
# build/scripts/install-bicep.sh, the single source of truth for that.
#
# Usage: ./gen-bicepconfig.sh <release-channel> <output-dir>
# Example: ./gen-bicepconfig.sh edge ./output

REL_CHANNEL=$1
OUTPUT_DIR=$2

if [ -z "$REL_CHANNEL" ]; then
  echo "Release channel is required. Please provide it as the first argument."
  exit 1
fi

if [ -z "$OUTPUT_DIR" ]; then
  echo "Output directory is required. Please provide it as the second argument."
  exit 1
fi

# Radius Bicep types uses latest tag
if [ "$REL_CHANNEL" = "edge" ]; then
  REL_CHANNEL="latest"
fi

# Create versioned bicepconfig.json
mkdir -p "${OUTPUT_DIR}"
cat <<EOF > "${OUTPUT_DIR}/bicepconfig.json"
{
  "extensions": {
    "radius": "br:biceptypes.azurecr.io/radius:${REL_CHANNEL}",
    "aws": "br:biceptypes.azurecr.io/aws:${REL_CHANNEL}"
  }
}
EOF
