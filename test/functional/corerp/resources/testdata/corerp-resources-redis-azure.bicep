import radius as radius

param magpieimage string
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'corerp-resources-redis-azure'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
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

resource redis 'Applications.Link/redisCaches@2022-03-15-privatepreview' = {
  name: 'redis-link'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    mode: 'resource'
    resource: '/subscriptions/85716382-7362-45c3-ae03-2126e459a123/resourceGroups/RadiusFunctionalTest/providers/Microsoft.Cache/Redis/redis-radiustest'
  }
}
