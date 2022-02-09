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
          source: redisCache.id
        }
      }
    }
  }
  resource redisCache 'redislabs.com.RedisCache' existing = {
    name: 'cool-cache'
  }
}

module redis 'br:radius.azurecr.io/starters/redis:latest' = {
  name: 'redis-module'
  params: {
    radiusApplication: app
    cacheName: 'cool-cache'
  }
}
