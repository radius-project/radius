import radius as radius

param magpieimage string
param environment string
param namespace string = 'default'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dpsb-mnl-app-old'
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dpsb-mnl-app-ctnr-old'
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
        appId: 'dpsb-mnl-app-ctnr-old'
        appPort: 3000
      }
    ]
  }
}

module redis 'modules/redis-selfhost.bicep' = {
  name: 'dpsb-mnl-redis-old-deployment'
  params: {
    name: 'dpsb-mnl-redis-old'
    namespace: namespace
    application: app.name
  }
}


resource pubsubBroker 'Applications.Link/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'dpsb-mnl-old'
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
