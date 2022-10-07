import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

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
      kind: SystemAssigned
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
    }
    volumes: {
      volkv: {
          kind: Persistent
          source: keyvaultVolume.id
          mountPath: '/var/secrets'
      }
    }
  }
}

@description('Specifies the value of the secret that you want to create.')
@secure()
param secretValue string

resource key_vault 'Microsoft.KeyVault/vaults@2021-04-01-preview' = {
  name: uniqueString('kv', resourceGroup().id)
  location: location
  properties: {
    enabledForTemplateDeployment: true
    tenantId: subscription().tenantId
    enableRbacAuthorization:true
    sku: {
      name: 'standard'
      family: 'A'
    }
  }
}

resource my_secret 'Microsoft.KeyVault/vaults/secrets@2021-04-01-preview' = {
  parent: key_vault
  name: 'mysecret'
  properties: {
    value: secretValue
    attributes:{
      enabled:true
    }
  }
}
