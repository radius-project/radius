#!/usr/bin/env bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
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
export AZURE_SERVICEBUS_RESOURCE_ID=$( jq -r '.properties.outputs.namespaceId.value' <<< "${resp}" ) 
export AZURE_MSSQL_RESOURCE_ID=$( jq -r '.properties.outputs.sqlServerId.value' <<< "${resp}" )
export AZURE_MSSQL_USERNAME=$adminUser
export AZURE_MSSQL_PASSWORD=$adminPassword
export AZURE_MONGODB_RESOURCE_ID=$( jq -r '.properties.outputs.mongoDatabaseId.value' <<< "${resp}" )
export AZURE_REDIS_RESOURCE_ID=$( jq -r '.properties.outputs.redisCacheId.value' <<< "${resp}" )
export AZURE_TABLESTORAGE_RESOURCE_ID=$( jq -r '.properties.outputs.tableStorageAccId.value' <<< "${resp}" )
export AZURE_COSMOS_MONGODB_ACCOUNT_ID=$( jq -r '.properties.outputs.cosmosMongoAccountID.value' <<< "${resp}" )
make test-functional-shared
make test-functional-msgrp
make test-functional-daprrp
make test-functional-datastoresrp
