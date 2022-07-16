import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' 

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'corerp-resources-redis-user-secrets'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'webapp'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        DBCONNECTION: redis.connectionString()
      }
      readinessProbe:{
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    connections: {
      redis: {
        source: redis.id
      }
    }
  }
}

resource redisContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'redis'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: 'redis:6.2'
      ports: {
        redis: {
          containerPort: 6379
          provides: redisRoute.id
        }
      }
    }
    connections: {}
  }
}

resource redisRoute 'Applications.Core/httproutes@2022-03-15-privatepreview' = {
  name: 'redis-route'
  location: 'global'
  properties: {
    application: app.id
  }
}

resource redis 'Applications.Connector/redisCaches@2022-03-15-privatepreview' = {
  name: 'redis'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    host: redisRoute.properties.hostname
    port: redisRoute.properties.port
    secrets: {
      connectionString: '${redisRoute.properties.hostname}:${redisRoute.properties.port}'
      password: ''
    }
  }
}
