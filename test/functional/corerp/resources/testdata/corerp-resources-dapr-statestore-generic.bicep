import radius as radius

param magpieimage string

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-dapr-statestore-generic'
  location: location
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'gnrc-sts-ctnr'
  location: location
  properties: {
    application: app.id
    connections: {
      daprstatestore: {
        source: statestore.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'gnrc-sts-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource redisContainer 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'gnrc-rds-ctnr'
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

resource redisRoute 'Applications.Core/httproutes@2023-04-15-preview' = {
  name: 'gnrc-rds-rte'
  location: 'global'
  properties: {
    application: app.id
  }
}

resource redis 'Applications.Link/redisCaches@2023-04-15-preview' = {
  name: 'gnrc-rds-rds'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    mode: 'values'
    host: redisRoute.properties.hostname
    port: redisRoute.properties.port
    secrets: {
      connectionString: '${redisRoute.properties.hostname}:${redisRoute.properties.port}'
      password: ''
    }
  }
}

resource statestore 'Applications.Link/daprStateStores@2023-04-15-preview' = {
  name: 'gnrc-sts'
  location: location
  properties: {
    application: app.id
    environment: environment
    mode: 'values'
    type: 'state.redis'
    metadata: {
      redisHost: '${redisRoute.properties.hostname}:${redisRoute.properties.port}'
      redisPassword: ''
    }
    version: 'v1'
  }
}
