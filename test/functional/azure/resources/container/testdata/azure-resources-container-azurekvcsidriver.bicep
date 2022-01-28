resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-azurekvcsidriver'

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        ports: {
          web: {
            containerPort: 80
          }
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
        volumes:{
          'my-kv':{
            kind: 'persistent'
            mountPath:'/tmpfs'
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
  name: 'kv${uniqueString('kv', resourceGroup().id)}'
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
