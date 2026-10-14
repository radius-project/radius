extension radius

param location string = resourceGroup().location

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

@description('Per-run seed used to ensure the Azure resource group name does not collide across concurrent CI runs that share a test subscription.')
param uniqueSeed string = ''

@description('Name of the Radius Environment.')
param envName string = 'corerp-resources-terraform-azrg-env'

@description('Name of the Radius Extender.')
param resourceName string = 'corerp-resources-terraform-azrg'

@description('Name of the Azure resource group created by the recipe.')
param azureResourceGroupName string = 'tfrg${uniqueString(resourceGroup().id, uniqueSeed)}'

@description('Test explicit provider configuration instead of the registered default provider.')
param useCustomProviderConfig bool = false

@description('Tenant of the registered Azure workload identity.')
param tenantId string = ''

@description('Client of the registered Azure workload identity.')
param clientId string = ''

@description('Projected workload identity token file in the Radius pod.')
param oidcTokenFilePath string = ''

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: envName
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: envName
    }
    providers: {
      azure: {
        scope: resourceGroup().id
      }
    }
    recipeConfig: useCustomProviderConfig ? {
      terraform: {
        providers: {
          azurerm: [
            {
              alias: 'custom'
              features: {}
              subscription_id: subscription().subscriptionId
              use_cli: false
              oidc_token_file_path: oidcTokenFilePath
              secrets: {
                tenant_id: {
                  source: providerSettings!.id
                  key: 'tenantId'
                }
              }
            }
          ]
        }
      }
      env: {
        ARM_USE_OIDC: 'true'
        // Explicit provider fields must override these invalid environment defaults.
        ARM_TENANT_ID: 'invalid-tenant'
        ARM_SUBSCRIPTION_ID: 'invalid-subscription'
        ARM_CLIENT_ID: 'invalid-client'
      }
      envSecrets: {
        ARM_CLIENT_ID: {
          source: providerSettings!.id
          key: 'clientId'
        }
      }
    } : {}
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/${useCustomProviderConfig ? 'azure-rg-custom-provider' : 'azure-rg'}.zip'
          parameters: {
            name: azureResourceGroupName
            location: location
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: appName
      }
    ]
  }
}

resource webapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: resourceName
  properties: {
    application: app.id
    environment: env.id
  }
}

resource providerSettings 'Applications.Core/secretStores@2023-10-01-preview' = if (useCustomProviderConfig) {
  name: '${resourceName}-provider-settings'
  properties: {
    resource: 'radius-system/${resourceName}-provider-settings'
    type: 'generic'
    data: {
      tenantId: {
        value: tenantId
      }
      clientId: {
        value: clientId
      }
    }
  }
}
