#!/bin/bash

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

set -e

GITEA_USERNAME=$1
GITEA_EMAIL=$2
GITEA_ACCESS_TOKEN_NAME=$3
# GITEA_PASSWORD should be set by environment variable

if [ -z "$GITEA_USERNAME" ]; then
  echo "GITEA_USERNAME is not set. Exiting..."
  exit 1
fi

if [ -z "$GITEA_EMAIL" ]; then
  echo "GITEA_EMAIL is not set. Exiting..."
  exit 1
fi

if [ -z "$GITEA_ACCESS_TOKEN_NAME" ]; then
  echo
  echo "GITEA_ACCESS_TOKEN_NAME is not set. Exiting..."
  exit 1
fi

# Add Gitea Helm chart repository
helm repo add gitea-charts https://dl.gitea.io/charts/
helm repo update

# Install Gitea from Helm chart
helm install gitea gitea-charts/gitea --namespace gitea --create-namespace -f .github/actions/install-gitea/gitea-config.yaml
kubectl wait --for=condition=available deployment/gitea -n gitea --timeout=120s

# Get the Gitea pod name
gitea_pod=$(kubectl get pods -n gitea -l app=gitea -o jsonpath='{.items[0].metadata.name}')

# Create a Gitea admin user
output=$(kubectl exec -it "$gitea_pod" -n gitea -- gitea admin user create --admin --username "$GITEA_USERNAME" --email "$GITEA_EMAIL" --password "$GITEA_PASSWORD" --must-change-password=false)
echo "$output"

# Generate an access token for the Gitea admin user
output=$(kubectl exec -it "$gitea_pod" -n gitea -- gitea admin user generate-access-token --username "$GITEA_USERNAME" --token-name "$GITEA_ACCESS_TOKEN_NAME"  --scopes "write:repository,write:user" --raw)
echo "$output"

echo "gitea-access-token=$output" >> "$GITHUB_OUTPUT"
