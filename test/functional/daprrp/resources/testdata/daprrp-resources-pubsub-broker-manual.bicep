import radius as radius

param magpieimage string
param environment string
param namespace string = 'default'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dpsb-manual-app'
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'dpsb-manual-app-ctnr'
  properties: {
    application: app.id
    connections: {
      daprpubsub: {
        source: pubsubBroker.id
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
        appId: 'dpsb-manual-app-ctnr'
        appPort: 3000
      }
    ]
  }
}


module redis '../../../shared/resources/testdata/modules/redis-selfhost.bicep' = {
  name: 'dpsb-manual-redis-deployment'
  params: {
    name: 'dpsb-manual-redis'
    namespace: namespace
    application: app.name
  }
}


resource pubsubBroker 'Applications.Dapr/pubSubBrokers@2023-10-01-preview' = {
  name: 'dpsb-manual'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'pubsub.redis'
    metadata: {
      redisHost: '${redis.outputs.host}:${redis.outputs.port}'
      redisPassword: ''
    }
    version: 'v1'
  }
}
