## Build Radius infrastructure to Azure

This includes the bicep template to build the infrastructure on Azure by deploying the following resources:

* Log Analytics Workspace for log
* Azure Monitor Workspace for metric 
* AKS Cluster
  * Installed extensions : Azure Keyvault CSI driver, Dapr
* Grafana dashboard
* Installed tools
  - cert-manager
  - Azure workload identity webhook handler

## Prerequisite

1. Azure CLI

## Steps

1. Ensure that you logged in Azure and select your subscription
    ```bash
    az login
    az account set -s [Subscription Id]
    ```
1. Create resource group
    ```bash
    az group create --location [Region Name] --resource-group [Resource Group Name]
    ```
1. Deploy main.bicep
    ```bash
    az deployment group create --resource-group [Resource Group Name] --template-file main.bicep --parameters grafanaAdminObjectId='[Admin Object Id]'
    ```
    > **How to find [Admin Object Id]**: You can use user or group id as a Grafana Admin. To find the object id, search admin user or group on [AAD Overview search](https://ms.portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/Overview) and get the object id.

## References

* https://github.com/Azure/prometheus-collector/blob/main/AddonBicepTemplate/AzureMonitorAlertsProfile.bicep
* https://github.com/Azure-Samples/aks-istio-addon-bicep/tree/main/bicep
