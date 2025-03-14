#!/bin/bash

REL_CHANNEL=$1
OUTPUT_DIR=$2

if [ "$REL_CHANNEL" = "edge" ]; then
  REL_CHANNEL="latest"
fi

mkdir -p $OUTPUT_DIR
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
