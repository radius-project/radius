resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-redis-managed'
  
  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        redis: {
          kind: 'redislabs.com/Redis'
          source: redis.id
        }
      }
    }
  }

  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }
}
