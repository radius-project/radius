#!/bin/bash
set -e

if [[ -z "$1" ]]
then
    echo "usage: deploy.sh <resource-group>"
    exit 1
fi

BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

az deployment group create \
    --resource-group "$1" \
    --files "$BASE_DIR/template.bicep"