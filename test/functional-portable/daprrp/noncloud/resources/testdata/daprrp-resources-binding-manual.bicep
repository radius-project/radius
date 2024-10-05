extension radius

param magpieimage string
param environment string
param namespace string = 'default'
param baseName string = 'dbd-manual'

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
      daprbinding: {
        source: binding.id
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
        appId: 'dbd-manual-ctnr'
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


resource binding 'Applications.Dapr/bindings@2023-10-01-preview' = {
  name: '${baseName}-dbd'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'bindings.redis'
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
