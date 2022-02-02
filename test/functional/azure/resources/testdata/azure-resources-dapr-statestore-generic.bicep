resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-dapr-statestore-generic'

  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      connections: {
        daprstatestore: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
  
  resource statestore 'dapr.io.StateStore@v1alpha3' = {
    name: 'statestore-generic'
    properties: {
      kind: 'generic'
      type: 'state.zookeeper'
      version: 'v1'
      metadata: {
        servers: 'zookeeper.default.svc.cluster.local:2181'
      }
    }
  }

  resource redis 'redislabs.com.RedisCache' = {
    name: 'myredis-connector'
    properties: {
      resource: azureRedis.id
    }
  }
}

resource azureRedis 'Microsoft.Cache/Redis@2019-07-01' = {
  name: 'myredis'
  location: 'westus2'
  properties: {
    sku: {
      capacity: 0
      family: 'C'
      name: 'Basic'
    }
  }
}


