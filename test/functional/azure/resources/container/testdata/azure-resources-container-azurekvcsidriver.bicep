resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-azurekvcsidriver'

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      connections: {
        backend: {
          kind: 'Http'
          source: backend_http.id
        }
      }
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        env: {
          SERVICE__BACKEND__HOST: backend_http.properties.host
          SERVICE__BACKEND__PORT: string(backend_http.properties.port)
        }
      }
    }
  }

  resource backend_http 'HttpRoute' = {
    name: 'backend'
  }

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'rynowak/backend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: backend_http.id
          }
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
  name: 'azure-kv-123'
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
