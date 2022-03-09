resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-redis-user-secrets'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpiego:latest'
        env: {
        }
      }
      connections: {
        redis: {
          kind: 'redislabs.com/Redis'
          source: redis.id
        }
      }
    }
  }

  resource redisContainer 'Container' = {
    name: 'redis'
    properties: {
      container: {
        image: 'redis:6.2'
        ports: {
          redis: {
            containerPort: 6379
            provides: redisRoute.id
          }
        }
      }
    }
  }

  resource redisRoute 'HttpRoute' = {
    name: 'redis-route'
    properties: {
      port: 6379
    }
  }

  resource redis 'redislabs.com.RedisCache@v1alpha3' = {
    name: 'redis'
    properties: {
      host: redisRoute.properties.host
      port: redisRoute.properties.port
      secrets: {
        connectionString: '${redisRoute.properties.host}:${redisRoute.properties.port}'
        password: ''
      }
    }
  }
}
