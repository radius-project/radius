# set app context 
import rad = {
  application: 'myapp'
}

# define existing, pre-deployed resources  
resource redis 'Microsoft.Cache/Redis@2019-07-01' existing = {
  name: 'myredis'
}

# define new resources for Radius to create & manage
resource container 'Container@v1alpha3' = {
  name: 'mycontainer'
  properties: {
    container: {
      image: 'myrepository/mycontainer:latest'
    }
    connections: {
      redis: {
        kind: 'Azure'
        source: redis.id
      }
    }
  }
}
