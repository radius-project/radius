import radius as radius

param magpieimage string
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-versioning'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'friendly-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        DBCONNECTION: redis.connectionString()
      }
      readinessProbe: {
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

resource redisContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'friendly-rds-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: 'redis:6.2'
      ports: {
        redis: {
          containerPort: 6379
        }
      }
    }
    connections: {}
  }
}

resource redis 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'friendly-rds-rds'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    host: 'friendly-rds-ctnr'
    port: 6379
    secrets: {
      connectionString: 'friendly-rds-ctnr:6379'
      password: ''
    }
  }
}
