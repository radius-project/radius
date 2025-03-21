#!/bin/bash

# This script installs the latest version of the Bicep CLI 
# and creates a configuration file for Bicep with the specified release channel.
# This is used to build the Bicep container image, and is called automatically
# by the `make build-bicep` and `make docker-build-bicep` commands.

# Usage: ./install-bicep.sh <release-channel> <output-dir>

REL_CHANNEL=$1
OUTPUT_DIR=$2

# Radius Bicep types uses latest tag
if [ "$REL_CHANNEL" = "edge" ]; then
  REL_CHANNEL="latest"
fi

# Check if curl is installed
if ! command -v curl &> /dev/null
then
    echo "curl could not be found, please install it first."
    exit 1
fi

# Create versioned bicepconfig.json
mkdir -p "$OUTPUT_DIR"
cat <<EOF > $OUTPUT_DIR/bicepconfig.json
{
  "experimentalFeaturesEnabled": {
    "extensibility": true
  },
  "extensions": {
    "radius": "br:biceptypes.azurecr.io/radius:${REL_CHANNEL}",
    "aws": "br:biceptypes.azurecr.io/aws:${REL_CHANNEL}"
  }
}
EOF

# Install latest version of Bicep CLI
curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-musl-x64
chmod +x bicep
mv bicep "$OUTPUT_DIR"/bicep
