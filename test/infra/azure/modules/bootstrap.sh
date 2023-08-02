#!/bin/bash

# Cert-manager variables
CertManagerVersion="v1.12.0"

az aks install-cli --only-show-errors

# Get AKS credentials
az aks get-credentials \
  --admin \
  --name $clusterName \
  --resource-group $resourceGroupName \
  --subscription $subscriptionId \
  --only-show-errors

echo "Installing Helm..."
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

echo "Installing cert-manager..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$CertManagerVersion/cert-manager.yaml

echo '{}' >$AZ_SCRIPTS_OUTPUT_PATH