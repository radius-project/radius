extension radius

param location string = resourceGroup().location

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

@description('Client ID for Azure.')
param clientID string

@description('Tenant ID for Azure.')
param tenantID string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-azrg-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-azrg-env'
    }
    providers: {
      azure: {
        scope: resourceGroup().id
      }
    }
    recipeConfig: {
      terraform: {
        providers: {
          azurerm: [ {
              alias: 'azure-test'
              features: {}
              subscription_id: subscription().subscriptionId
              use_oidc: true
              oidc_token_file_path: '/var/run/secrets/azure/tokens/azure-identity-token'
              use_cli: false
              secrets: {
                tenant_id: {
                  source: secretstore.id
                  key: 'tenantID'
                }
                client_id: {
                  source: secretstore.id
                  key: 'clientID'
                }
              }
            } ]
        }
      }
      // env: {
      //   ARM_USE_AKS_WORKLOAD_IDENTITY: 'true'
      //   ARM_USE_CLI: 'false'
      // }
      // envSecrets: {
      //   ARM_CLIENT_ID: {
      //     source: secretstore.id
      //     key: 'clientID'
      //   }
      // }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/azure-rg.zip'
          parameters: {
            name: 'tfrg${uniqueString(resourceGroup().id)}'
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
  name: 'corerp-resources-terraform-azrg'
  properties: {
    application: app.id
    environment: env.id
  }
}

resource secretstore 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-azrg-secretstore'
  properties: {
    resource: 'corerp-resources-terraform-azrg/secretstore'
    type: 'generic'
    data: {
      tenantID: {
        value: tenantID
      }
      clientID: {
        value: clientID
      }
    }
  }
}
