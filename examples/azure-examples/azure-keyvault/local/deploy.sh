#!/bin/bash
set -e

if [[ -z "$SUBSCRIPTION_ID" ]]
then
    echo "SUBSCRIPTION_ID not set"
fi

if [[ -z "$RESOURCE_GROUP" ]]
then
    echo "RESOURCE_GROUP not set"
fi

if [[ -z "$RESOURCE_PROVIDER" ]]
then
    echo "RESOURCE_PROVIDER not set"
fi

BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
URL_BASE="http://localhost:5000/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RESOURCE_GROUP/providers/Microsoft.CustomProviders/resourceProviders/$RESOURCE_PROVIDER"

APPLICATION="azure-keyvault"
COMPONENTS=("kv" "app")
DEPLOYMENTS=("default")

echo "creating application $APPLICATION"
http PUT \
    "$URL_BASE/Applications/$APPLICATION" \
    "@$BASE_DIR/application.json" \
    --check-status

for i in ${COMPONENTS[@]}; do
  echo "creating component $i"
  http PUT \
    "$URL_BASE/Applications/$APPLICATION/Components/$i" \
    "@$BASE_DIR/component-$i.json" \
    --check-status
done

for i in ${DEPLOYMENTS[@]}; do
  echo "creating deployment $i"
  http PUT \
    "$URL_BASE/Applications/$APPLICATION/Deployments/$i" \
    @"$BASE_DIR/deployment-$i.json" \
    --check-status
done