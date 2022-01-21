resource cache 'Microsoft.Cache/Redis@2019-07-01' = {
  name: 'mycache'
  location: 'westus2'
  properties: {
    sku: {
      capacity: 0
      family: 'C'
      name: 'Basic'
    }
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myimage'
        env: {
          REDIS_HOST: cache.properties.hostName
        }
      }
      connections: {
        redis: {
          kind: 'azure'
          source: cache.id
          roles: [
            'Redis Cache Contributor'
          ]
        }
      }
    }
  }
}
