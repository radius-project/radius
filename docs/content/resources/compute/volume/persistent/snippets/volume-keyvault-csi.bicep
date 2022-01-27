resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        volumes: {
          myPersistentVolume: {
            kind: 'persistent'
            mountPath: '/tmpfs'
            source: myshare.id
            rbac: 'read'
          }
        }
      }
    }
  }
  resource myshare 'Volume' = {
    name: 'myshare'
    properties:{
      kind: 'azure.com.keyvault'
      managed:false
      resource: key_vault.id
      secrets: {
        mysecret: {
          name: 'mysecret'
          encoding: 'utf-8'
        }
      }
    }
  }
}

@description('Specifies the value of the secret that you want to create.')
@secure()
param secretValue string

resource key_vault 'Microsoft.KeyVault/vaults@2021-04-01-preview' = {
  name: 'kv-${uniqueString('kv', resourceGroup().id)}'
  location: resourceGroup().location
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
