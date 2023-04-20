import radius as radius

param magpieimage string
param environment string
param redisresourceid string

resource app 'Applications.Core/applications@2023-04-15-preview'  = {
  name: 'corerp-resources-redis-azure'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'redis-azure-app-ctnr'
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

resource redis 'Applications.Link/redisCaches@2023-04-15-preview' = {
  name: 'redis-link'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    mode: 'resource'
    resource: redisresourceid
  }
}
