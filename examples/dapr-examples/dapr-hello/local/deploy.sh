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

if [[ -z "$REDIS_HOST" ]]
then
    echo "REDIS_HOST not set"
fi

if [[ -z "$REDIS_PASSWORD" ]]
then
    echo "REDIS_PASSWORD not set"
fi

BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
URL_BASE="http://localhost:5000/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RESOURCE_GROUP/providers/Microsoft.CustomProviders/resourceProviders/$RESOURCE_PROVIDER"

APPLICATION="dapr-hello"
COMPONENTS=("nodeapp" "pythonapp") # statestore and pubsub need to have some data injected
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

for i in "statestore" "pubsub"; do
  echo "creating component $i"
  cat "$BASE_DIR/component-$i.json" | jq ". |.properties.workload.metadata[1].value=\"$REDIS_HOST\" |.properties.workload.metadata[2].value=\"$REDIS_PASSWORD\"" | \
    http PUT \
        "$URL_BASE/Applications/$APPLICATION/Components/$i" \
        --check-status
done

for i in ${DEPLOYMENTS[@]}; do
  echo "creating deployment $i"
  http PUT \
    "$URL_BASE/Applications/$APPLICATION/Deployments/$i" \
    @"$BASE_DIR/deployment-$i.json" \
    --check-status
done