// This is the template to create keyvault volume with workload identity specified in radius volume resource.
//
// 1. Create the keyvault resource and test secret
// 2. Create the environment with workload identity and root scope where radius will create managed identity.
// 3. Create Keyvault volume.
// 4. Create container which associated keyvault volume.

import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the scope of azure resources.')
param rootScope string = resourceGroup().id

@description('Specifies the environment for resources.')
#disable-next-line no-hardcoded-env-urls
param oidcIssuer string = 'https://radiusoidc.blob.core.windows.net/kubeoidc/'


resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-azure-workload-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-azure-workload-env'
      identity: {
        kind: 'azure.com.workload'
        oidcIssuer: oidcIssuer
      }
    }
    providers: {
      azure: {
        scope: rootScope
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-volume-azure-keyvault'
  location: location
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-volume-azure-keyvault-app'
      }
    ]
  }
}

resource keyvaultVolContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'volume-azkv-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
      }
      volumes: {
        volkv: {
            kind: 'persistent'
            source: keyvaultVolume.id
            mountPath: '/var/secrets'
        }
      }
    }
  }
}

resource keyvaultVolume 'Applications.Core/volumes@2023-10-01-preview' = {
  name: 'volume-azkv'
  location: location
  properties: {
    application: app.id
    kind: 'azure.com.keyvault'
    // Due to the soft-delete production of keyvault, this test uses the existing test keyvault.
    resource: '/subscriptions/85716382-7362-45c3-ae03-2126e459a123/resourceGroups/RadiusFunctionalTest/providers/Microsoft.KeyVault/vaults/radiuskvvoltest'
    secrets: {
      mysecret: {
        name: 'mysecret'
      }
    }
  }
}

/*
// Due to the soft-delete production of keyvault, this test uses the existing test keyvault.
// If you want to create keyvault while deploying this bicep template, please uncomment the below resource template.
resource azTestKeyvault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'radkvt${uniqueString(resourceGroup().name)}'
  location: resourceGroup().location
  tags: {
    radiustest: 'corerp-resources-key-vault'
  }
  properties: {
    enabledForTemplateDeployment: true
    tenantId: keyvaultTenantID
    enableRbacAuthorization:true
    enableSoftDelete: false
    softDeleteRetentionInDays: 7
    sku: {
      name: 'standard'
      family: 'A'
    }
  }
}

resource mySecret 'Microsoft.KeyVault/vaults/secrets@2022-07-01' = {
  parent: azTestKeyvault
  name: 'mysecret'
  properties: {
    value: mySecretValue
    attributes:{
      enabled:true
    }
  }
}
*/

