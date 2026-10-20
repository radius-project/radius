#!/bin/bash

# Cert-manager variables
CertManagerVersion="v1.20.3"

az aks install-cli --only-show-errors

# Get AKS credentials. clusterName, resourceGroupName, and subscriptionId are
# injected as environment variables by the Azure deploymentScript resource.
# shellcheck disable=SC2154
az aks get-credentials \
  --admin \
  --name $clusterName \
  --resource-group $resourceGroupName \
  --subscription $subscriptionId \
  --only-show-errors

echo "Installing Helm..."
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4 | bash

echo "Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$CertManagerVersion/cert-manager.yaml

echo '{}' >$AZ_SCRIPTS_OUTPUT_PATH
