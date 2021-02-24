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

URL_BASE="http://localhost:5000/subscriptions/$SUBSCRIPTION_ID/resourceGroups/$RESOURCE_GROUP/providers/Microsoft.CustomProviders/resourceProviders/$RESOURCE_PROVIDER"
BASE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

http PUT \
    "$URL_BASE/Applications/frontend-backend" \
    "@$BASE_DIR/application.json" \
    --check-status
http PUT \
    "$URL_BASE/Applications/frontend-backend/Components/frontend" \
    "@$BASE_DIR/component-frontend.json" \
    --check-status
http PUT \
    "$URL_BASE/Applications/frontend-backend/Components/backend" \
    "@$BASE_DIR/component-backend.json" \
    --check-status

FRONTEND_REV="$(http GET "$URL_BASE/Applications/frontend-backend/Components/frontend" --check-status --print=b | jq .revision)"
BACKEND_REV="$(http GET "$URL_BASE/Applications/frontend-backend/Components/backend" --check-status --print=b | jq .revision)"

cat "$BASE_DIR/deployment-default.json" | jq ". |.properties.components[0].revision=$FRONTEND_REV |.properties.components[1].revision=$BACKEND_REV" | \
    http PUT \
        "$URL_BASE/Applications/frontend-backend/Deployments/default" \
        --check-status