//RESOURCE
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
//RESOURCE

resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'myapp'

  //CONNECTOR
  resource redis 'redislabs.com.RedisCache' = {
    name: 'myredis-connector'
    properties: {
      resource: azureRedis.id
    }
  }
  //CONNECTOR

  //CONTAINER
  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myrepo/myimage'
      }
      connections: {
        inventory: {
          kind: 'redislabs.com/Redis'
          source: redis.id
        }
      }
    }
  }
  //CONTAINER

}
