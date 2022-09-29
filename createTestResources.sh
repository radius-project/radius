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
resp=`az group deployment create --resource-group $resourcegroup --template-file createTestAzureResources.bicep`
resp=`cat resourceId.json`
outputResource=$( jq -r  '.properties.outputResources' <<< "${resp}" ) 
echo "${outputResource}" > resourceId.txt