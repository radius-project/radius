#!/usr/bin/env bash

# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------
usage(){
  echo -e "$0 requires <resourcegroup_name>\n"
  exit 1
}

if [ $# -lt 1 ];then
  usage
fi

resourcegroup=$1
# create test resources for local testing using the az group deploy
resp=`az group deployment create --resource-group $resourcegroup --template-file createAzureResources.bicep`
export SERVICEBUS_RESOURCE_ID=$( jq -r '.properties.outputs.namespace.value' <<< "${resp}" ) 
export MSSQL_RESOURCE_ID=$( jq -r '.properties.outputs.sqlServerId.value' <<< "${resp}" )
export MSSQL_USERNAME=$( jq -r '.properties.outputs.msqlUserName.value' <<< "${resp}" )
export MSSQL_PASSWORD=$( jq -r '.properties.outputs.msqlPassword.value' <<< "${resp}" )
export MONGODB_RESOURCE_ID=$( jq -r '.properties.outputs.mongoDatabaseId.value' <<< "${resp}" )
export REDIS_RESOURCE_ID=$( jq -r '.properties.outputs.redisCacheId.value' <<< "${resp}" )
export TABLESTORAGE_RESOURCE_ID=$( jq -r '.properties.outputs.tableStorageAccId.value' <<< "${resp}" )
make test-functional-corerp
