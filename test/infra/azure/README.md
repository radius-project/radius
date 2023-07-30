## Build Radius infrastructure to Azure

This directory includes the Bicep templates to deploy the following resources on Azure for running Radius:

* Log Analytics Workspace for log
* Azure Monitor Workspace for metric 
* AKS Cluster
  * Installed extensions: Azure Keyvault CSI driver, Dapr
* Grafana dashboard
* Installed tools
  - cert-manager v1.20.0
  - Azure workload identity mutating admission webhook controller v1.1.0

## Prerequisite

1. [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli)
2. [Azure subscription](https://azure.com) to which you have a Owner/Contributor role

## Steps

1. Log in to Azure and select your subscription:
    ```bash
    az login
    az account set -s [Subscription Id]
    ```

1. Enable `Microsoft.ContainerService/EnableImageCleanerPreview` feature flag
    
    This cleans up unused container images in each node, which can occur the security vulnerability. Visit https://aka.ms/aks/image-cleaner to learn more about image cleaner.

    ```bash
    # Register feature flag.
    az feature register --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview"

    # Ensure that the feature flag is `Registered`.
    az feature show --namespace "Microsoft.ContainerService" --name "EnableImageCleanerPreview"

    # Re-register resource provider.
    az provider register --namespace Microsoft.ContainerService
    ```

    > Note: When you enable the feature flag first in your subscription, it will take some time to be propagated.

1. Create resource group:
    ```bash
    az group create --location [Location Name] --resource-group [Resource Group Name]
    ```
    - **[Location Name]**: Specify the location of the resource group. This location will be used as the default location for the resources in the template.
    - **[Resource Group Name]**: Provide a name for the resource group where the template will be deployed.

1. Deploy main.bicep:
    ```bash
    az deployment group create --resource-group [Resource Group Name] --template-file main.bicep --parameters grafanaAdminObjectId='[Admin Object Id]'
    ```
    - **[Admin Object Id]**: Set the object ID of the Grafana Admin user or group. To find the object id, search for the admin user or group name on [AAD Portal Overview search box](https://portal.azure.com/#view/Microsoft_AAD_IAM/ActiveDirectoryMenuBlade/~/Overview) and get the object id or run `az ad signed-in-user show` to get your own user object id.

## References

* https://github.com/Azure/prometheus-collector/blob/main/AddonBicepTemplate/AzureMonitorAlertsProfile.bicep
* https://github.com/Azure-Samples/aks-istio-addon-bicep/tree/main/bicep
