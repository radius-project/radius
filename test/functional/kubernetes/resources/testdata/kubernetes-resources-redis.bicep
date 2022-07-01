param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param port int = 6379

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-redis'

  resource webapp 'Container' = {
    name: 'webapp'
    properties: {
      container: {
        image: magpieimage
        readinessProbe:{
          kind: 'httpGet'
          containerPort: 3000
          path: '/healthz'
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
    name: 'redis-container'
    properties: {
      container: {
        image: 'redis:6.2'
        ports: {
          redis: {
            containerPort: port
            provides: redisRoute.id
          }
        }
      }
    }
  }

  resource redisRoute 'HttpRoute' = {
    name: 'redis-route'
    properties: {
      port: port
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
