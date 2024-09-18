extension radius

param magpieimage string
param environment string
param namespace string = 'default'
param baseName string = 'dcs-manual'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: baseName
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: '${baseName}-ctnr'
  properties: {
    application: app.id
    connections: {
      daprconfigurationstore: {
        source: configStore.id
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
        appId: 'dcs-manual-ctnr'
        appPort: 3000
      }
    ]
  }
}


module redis '../../../../../../test/testrecipes/modules/redis-selfhost.bicep' = {
  name: '${baseName}-redis-deployment'
  params: {
    name: '${baseName}-redis'
    namespace: namespace
    application: app.name
  }
}


resource configStore 'Applications.Dapr/configurationStores@2023-10-01-preview' = {
  name: '${baseName}-dcs'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'configuration.redis'
    metadata: {
      redisHost: {
        value: '${redis.outputs.host}:${redis.outputs.port}'
      }
      redisPassword: {
        value: ''
      }
    }
    version: 'v1'
  }
}
