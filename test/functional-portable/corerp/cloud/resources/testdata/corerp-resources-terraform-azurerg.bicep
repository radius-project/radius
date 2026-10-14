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
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/azure-rg.zip'
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
