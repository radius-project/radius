## Build Radius infrastructure to Azure

This includes the bicep template to build the infrastructure on Azure by deploying the following resources:

* Log Analytics Workspace for log
* Azure Monitor Workspace for metric 
* AKS Cluster
  * Installed extensions : Azure Keyvault CSI driver, Dapr
* Grafana dashboard

## Steps

1. Ensure that you logged in Azure and select your subscription
    ```bash
    az login
    az account set -s <Subscription Id>
    ```
1. Create resource group
    ```bash
    az group create --location <Region Name> --resource-group <Resource Group Name>
    ```
1. Deploy main.bicep
    ```bash
    az deployment group create --resource-group <Resource Group Name> --template-file main.bicep --parameters grafanaAdminObjectId='<Admin Object Id>'
    ```

## References

* https://github.com/Azure/prometheus-collector/blob/main/AddonBicepTemplate/AzureMonitorAlertsProfile.bicep
* https://github.com/Azure-Samples/aks-istio-addon-bicep/tree/main/bicep
