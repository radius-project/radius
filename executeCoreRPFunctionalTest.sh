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
echo $resourcegroup
# set the username and password for msqlDB to be passed as parameters to the bicep template
adminUser='coolUser'
adminPassword=$(uuidgen)
resp=`az deployment group create --resource-group $resourcegroup --template-file createAzureTestResources.bicep --parameters 'adminUsername=$adminUser' --parameters 'adminPassword=$adminPassword'`
cat resp
export SERVICEBUS_RESOURCE_ID=$( jq -r '.properties.outputs.namespaceId.value' <<< "${resp}" ) 
export MSSQL_RESOURCE_ID=$( jq -r '.properties.outputs.sqlServerId.value' <<< "${resp}" )
export MSSQL_USERNAME=$adminUser
export MSSQL_PASSWORD=$adminPassword
export MONGODB_RESOURCE_ID=$( jq -r '.properties.outputs.mongoDatabaseId.value' <<< "${resp}" )
export REDIS_RESOURCE_ID=$( jq -r '.properties.outputs.redisCacheId.value' <<< "${resp}" )
export TABLESTORAGE_RESOURCE_ID=$( jq -r '.properties.outputs.tableStorageAccId.value' <<< "${resp}" )
export DOCUMENTDB_RESOURCE_ID=$( jq -r '.properties.outputs.cosmosMongoAccountID.value' <<< "${resp}" )
make test-functional-corerp
