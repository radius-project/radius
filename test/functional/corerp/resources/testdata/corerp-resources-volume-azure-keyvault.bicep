import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the value of the secret that you want to create.')
@secure()
param mySecretValue string = newGuid()

@description('Specifies the value of tenantId.')
param keyvaultTenantID string = subscription().tenantId

@description('Specifies the value of keyvault admin role.')
param keyvaultAdminRoleDefinitionId string = '/providers/Microsoft.Authorization/roleDefinitions/00482a5a-887f-4fb3-b363-3b7fe8e74483'

@description('Specifies the value of System assigned managed identity.')
param systemIdentityId string = '2662fdc5-e7d9-4a4f-b79a-522122f282ba'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-volume-azkv'
  location: location
  properties: {
    environment: environment
  }
}

resource keyvaultVolume 'Applications.Core/volumes@2022-03-15-privatepreview' = {
  name: 'volume-azkv'
  location: location
  properties: {
    application: app.id
    kind: 'azure.com.keyvault'
    identity: {
      kind: 'SystemAssigned'
    }
    resource: key_vault.id
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

resource key_vault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'kv-volume-1'
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

resource roleAssignment 'Microsoft.Authorization/roleAssignments@2020-04-01-preview' = {
  scope: key_vault
  name: guid(key_vault.id, systemIdentityId, keyvaultAdminRoleDefinitionId)
  properties: {
    roleDefinitionId: keyvaultAdminRoleDefinitionId
    principalId: systemIdentityId
    principalType: 'ServicePrincipal'
  }
}

resource my_secret 'Microsoft.KeyVault/vaults/secrets@2022-07-01' = {
  parent: key_vault
  name: 'mysecret'
  properties: {
    value: mySecretValue
    attributes:{
      enabled:true
    }
  }
}
