// Define existing, pre-deployed resources 
resource redis 'Microsoft.Cache/Redis@2019-07-01' existing = {
  name: 'myredis'
}

// Define services and connection to existing resource
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'app'
  
  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myrepository/mycontainer:latest'
      }
      connections: {
        redis: {
          kind: 'azure'
          source: redis.id
        }
      }
    }
  }
  
}
