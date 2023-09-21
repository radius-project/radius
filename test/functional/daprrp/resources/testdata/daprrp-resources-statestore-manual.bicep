import radius as radius

param magpieimage string
param environment string
param namespace string = 'default'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'daprrp-rs-statestore-manual'
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'dapr-sts-manual-ctnr'
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


module redis '../../../shared/resources/testdata/modules/redis-selfhost.bicep' = {
  name: 'dapr-sts-manual-redis-deployment'
  params: {
    name: 'dapr-sts-manual-redis'
    namespace: namespace
    application: app.name
  }
}


resource statestore 'Applications.Dapr/stateStores@2023-10-01-preview' = {
  name: 'dapr-sts-manual'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'state.redis'
    metadata: {
      redisHost: '${redis.outputs.host}:${redis.outputs.port}'
      redisPassword: ''
    }
    version: 'v1'
  }
}
