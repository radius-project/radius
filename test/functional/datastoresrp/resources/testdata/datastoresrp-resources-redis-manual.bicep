import radius as radius

param magpieimage string
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'dsrp-resources-redis-manual'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'rds-app-ctnr'
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
  name: 'rds-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: 'redis:6.2'
      ports: {
        redis: {
          containerPort: 6379
          port: 80
        }
      }
    }
    connections: {}
  }
}



resource redis 'Applications.Datastores/redisCaches@2022-03-15-privatepreview' = {
  name: 'rds-rds'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    host: 'rds-ctnr'
    port: 80
    secrets: {
      connectionString: 'rds-ctnr:6379'
      password: ''
    }
  }
}
