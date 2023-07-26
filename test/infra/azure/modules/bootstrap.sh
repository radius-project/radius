#!/bin/bash

# Azure Workload identity webhook admission controller variables 
AZWIRepoName="azure-workload-identity"
AZWINamespace="azure-workload-identity-system"
AZWIReleaseName="workload-identity-webhook"
AZWIChartName="workload-identity-webhook"
AZWIVersion="1.1.0"
AZWIRepoUrl="https://azure.github.io/azure-workload-identity/charts"

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

echo "Installing Azure Workload Identity Webhook..."
helm repo add $AZWIRepoName $AZWIRepoUrl
helm repo update
helm install $AZWIReleaseName $AZWIRepoName/$AZWIChartName \
    --namespace $AZWINamespace \
    --create-namespace \
    --version $AZWIVersion \
    --set azureTenantID="$tenantId"

echo '{}' >$AZ_SCRIPTS_OUTPUT_PATH