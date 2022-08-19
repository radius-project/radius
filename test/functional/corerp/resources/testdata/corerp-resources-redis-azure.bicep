import radius as radius

param magpieimage string
param environment string

param location string = resourceGroup().location
param resourceIdentifier string = newGuid()

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

resource redis 'Applications.Connector/redisCaches@2022-03-15-privatepreview' = {
  name: 'redis-connector'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resource: redisCache.id
  }
}

resource redisCache 'Microsoft.Cache/redis@2020-12-01' = {
  name: 'redis-${resourceIdentifier}'
  location: location
  properties: {
    enableNonSslPort: false
    minimumTlsVersion: '1.2'
    sku: {
      family: 'C'
      capacity: 1
      name: 'Basic'
    }
  }
}
