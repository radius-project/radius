import radius as radius

param magpieimage string
param environment string
param namespace string = 'default'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dpsb-manual-app-old'
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dpsb-manual-app-ctnr-old'
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
        appId: 'dpsb-manual-app-old'
        appPort: 3000
      }
    ]
  }
}


module redis 'modules/redis-selfhost.bicep' = {
  name: 'dpsb-manual-redis-deployment-old'
  params: {
    name: 'dpsb-manual-redis-old'
    namespace: namespace
    application: app.name
  }
}


resource pubsubBroker 'Applications.Link/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'dpsb-manual-old'
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
