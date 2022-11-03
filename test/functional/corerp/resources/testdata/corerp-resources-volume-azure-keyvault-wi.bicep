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
param rootScope string

@description('Specifies the environment for resources.')
param oidcIssuer string = 'https://radiusoidc.blob.core.windows.net/kubeoidc/'

@description('Specifies the value of the secret that you want to create.')
@secure()
param mySecretValue string = newGuid()

@description('Specifies the value of tenantId.')
param keyvaultTenantID string = subscription().tenantId


resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
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

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-volume-azkv'
  location: location
  properties: {
    environment: env
  }
}

resource keyvaultVolume 'Applications.Core/volumes@2022-03-15-privatepreview' = {
  name: 'volume-azkv'
  location: location
  properties: {
    application: app.id
    kind: 'azure.com.keyvault'
    resource: azTestKeyvault.id
    secrets: {
      mysecret: {
        name: 'mysecret'
      }
    }
  }
}

resource keyvaultVolContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
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

// Prepare Azure resources - User assigned managed identity, keyvault, and role assignment.
resource kvVolIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2022-01-31-preview' = {
  name: 'kv-volume-mi'
  location: location
}

resource azTestKeyvault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'kv-volume'
  location: location
  tags: {
    radiustest: 'corerp-resources-key-vault'
  }
  properties: {
    enabledForTemplateDeployment: true
    tenantId: keyvaultTenantID
    enableRbacAuthorization:true
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
