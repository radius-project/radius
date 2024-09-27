extension radius

param magpieimage string
param environment string
param namespace string = 'default'
param baseName string = 'dapr-scopes-manual'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: baseName
  properties: {
    environment: environment
  }
}

resource ok 'Applications.Core/containers@2023-10-01-preview' = {
  name: '${baseName}-ctnr-ok'
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
        appId: '${baseName}-ctnr-ok'
        appPort: 3000
      }
    ]
  }
}

// This one will fail its healthcheck because it cannot access the state store
// as it is not in the state store scopes
resource failing 'Applications.Core/containers@2023-10-01-preview' = {
  name: '${baseName}-ctnr-ko'
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
        failureThreshold: 3
        periodSeconds: 10
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: '${baseName}-ctnr-ko'
        appPort: 3000
      }
    ]
  }

}


module redis '../../../../../../test/testrecipes/modules/redis-selfhost.bicep' = {
  name: '${baseName}-redis-deployment'
  params: {
    name: '${baseName}-manual-redis'
    namespace: namespace
    application: app.name
  }
}


resource statestore 'Applications.Dapr/stateStores@2023-10-01-preview' = {
  name: '${baseName}-sts'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'state.redis'
    metadata: {
      redisHost: {
        value: '${redis.outputs.host}:${redis.outputs.port}'
      }
      redisPassword: {
        value: ''
      }
    }
    version: 'v1'
    scopes: ['${baseName}-ctnr-ok']
  }
}
