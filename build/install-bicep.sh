#!/bin/bash

# This script installs the latest version of the Bicep CLI 
# and creates a configuration file for Bicep with the specified release channel.
# This is used to build the Bicep container image, and is called automatically
# by the `make build-bicep` and `make docker-build-bicep` commands.

# Usage: ./install-bicep.sh <release-channel> <output-dir> <arch>
# Example: ./install-bicep.sh edge ./output amd64

# Also, the script accepts the following environment variables:
# - RADIUS_BICEP_TYPES_REGISTRY: The registry for the Radius Bicep types. Default is "br:biceptypes.azurecr.io".
# - RADIUS_BICEP_TYPES_VERSION: The version for the Radius Bicep types. Default is "latest".
# - AWS_BICEP_TYPES_REGISTRY: The registry for the AWS Bicep types. Default is "br:biceptypes.azurecr.io".
# - AWS_BICEP_TYPES_VERSION: The version for the AWS Bicep types. Default is "latest".

REL_CHANNEL=$1
OUTPUT_DIR=$2
ARCH=$3

if [ -z "$REL_CHANNEL" ]; then
  echo "Release channel is required. Please provide it as the first argument."
  exit 1
fi

if [ -z "$OUTPUT_DIR" ]; then
  echo "Output directory is required. Please provide it as the second argument."
  exit 1
fi

if [ -z "$ARCH" ]; then
  echo "Architecture is required. Please provide it as the third argument."
  exit 1
fi

# Radius Bicep types uses latest tag
if [ "$REL_CHANNEL" = "edge" ]; then
  REL_CHANNEL="latest"
fi

RADIUS_BICEP_TYPES_REGISTRY=${RADIUS_BICEP_TYPES_REGISTRY:-"br:biceptypes.azurecr.io"}
RADIUS_BICEP_TYPES_VERSION=${RADIUS_BICEP_TYPES_VERSION:-$REL_CHANNEL}
AWS_BICEP_TYPES_REGISTRY=${AWS_BICEP_TYPES_REGISTRY:-"br:biceptypes.azurecr.io"}
AWS_BICEP_TYPES_VERSION=${AWS_BICEP_TYPES_VERSION:-$REL_CHANNEL}

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
    "radius": "br:${RADIUS_BICEP_TYPES_REGISTRY}/radius:${RADIUS_BICEP_TYPES_VERSION}",
    "aws": "br:${AWS_BICEP_TYPES_REGISTRY}/aws:${AWS_BICEP_TYPES_VERSION}"
  }
}
EOF

# Bicep CLI uses x64 or arm64
BICEP_ARCH="x64"
if [ "$ARCH" = "arm64" ]; then
  BICEP_ARCH="arm64"
fi

curl -Lo bicep https://github.com/Azure/bicep/releases/latest/download/bicep-linux-$BICEP_ARCH
if [ $? -ne 0 ]; then
  echo "Failed to download Bicep CLI. Please check your internet connection or the URL."
  exit 1
fi

chmod +x bicep
mv bicep "$OUTPUT_DIR"/bicep
