resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-redis-managed'
  
  resource webapp 'Container' = {
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

  resource redis 'redislabs.com.RedisCache' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }
}
