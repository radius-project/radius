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

APPLICATION="azure-keyvault"
COMPONENTS=("kv" "app")
DEPLOYMENTS=("default")

for i in ${COMPONENTS[@]}; do
  echo "deleting component $i"
  http DELETE "$URL_BASE/Applications/$APPLICATION/Components/$i" --check-status 
done

for i in ${DEPLOYMENTS[@]}; do
  echo "deleting deployment $i"
  http DELETE "$URL_BASE/Applications/$APPLICATION/Deployments/$i" --check-status 
done

echo "deleting application $APPLICATION"
http DELETE "$URL_BASE/Applications/$APPLICATION" --check-status 