resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myregistry/myimage'
        env: {
          REDIS_CS: redis.outputs.redisCache.connectionString()
        }
      }
      connections: {
        cache: {
          kind: 'redislabs.com/Redis'
          source: redis.outputs.redisCache.id
        }
      }
    }
  }
}

module redis 'br:radius.azurecr.io/starters/redis:latest' = {
  name: 'redis'
  params: {
    radiusApplication: app
  }
}
