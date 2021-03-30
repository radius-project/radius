#!/bin/bash
set -e

if [[ -z "$1" ]]
then
    echo "usage: delete.sh <resource-group>"
    exit 1
fi

RESOURCE_GROUP_ID="$(az group show --resource-group $1 --query 'id' --output tsv)"
URL_BASE="$RESOURCE_GROUP_ID/providers/Microsoft.CustomProviders/resourceProviders/radius/Applications"
VERSION="api-version=2018-09-01-preview"

APPLICATION="dapr-hello"
COMPONENTS=("nodeapp" "pubsub" "pythonapp" "statestore")
DEPLOYMENTS=("default")

for i in ${COMPONENTS[@]}; do
  echo "deleting component $i"
  az rest --method DELETE --uri "$URL_BASE/Applications/$APPLICATION/Components/default?$VERSION"
done

for i in ${DEPLOYMENTS[@]}; do
  echo "deleting deployment $i"
  az rest --method DELETE --uri "$URL_BASE/Applications/$APPLICATION/Deployments/default?$VERSION"
done

echo "deleting application $APPLICATION"
az rest --method DELETE --uri "$URL_BASE/Applications/$APPLICATION?VERSION" --check-status