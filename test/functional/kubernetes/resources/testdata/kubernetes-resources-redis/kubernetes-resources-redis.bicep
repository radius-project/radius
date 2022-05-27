import kubernetes from kubernetes

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource redisService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'redis-svc'
    namespace: 'kubernetes-resources-redis'
  }
}

resource redisSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'redis-pw'
    namespace: 'kubernetes-resources-redis'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-redis'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: magpieimage
        env: {
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
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

  resource redisRoute 'HttpRoute' = {
    name: 'redis-route'
    properties: {
      port: 80
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
