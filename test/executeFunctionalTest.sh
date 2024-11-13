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
usage() {
  echo -e "$0 requires <resourcegroup_name>\n"
  exit 1
}

if [ $# -lt 1 ]; then
  usage
fi

resourcegroup=$1
echo $resourcegroup

resp=$(az deployment group create --resource-group $resourcegroup --template-file createAzureTestResources.bicep)
cat resp

export AZURE_COSMOS_MONGODB_ACCOUNT_ID=$(jq -r '.properties.outputs.cosmosMongoAccountID.value' <<<"${resp}")
make test-functional-corerp
make test-functional-msgrp
make test-functional-daprrp
make test-functional-datastoresrp
