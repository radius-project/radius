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

http DELETE "$URL_BASE/Applications/frontend-backend/Components/frontend" --check-status 
http DELETE "$URL_BASE/Applications/frontend-backend/Components/backend" --check-status 
http DELETE "$URL_BASE/Applications/frontend-backend/Deployments/default" --check-status 
http DELETE "$URL_BASE/Applications/frontend-backend" --check-status 