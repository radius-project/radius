extension radius

param magpieimage string
param environment string
param namespace string = 'default'
param baseName string = 'dpsb-manual-secret'
@secure()
param redisPassword string = ''
param secretName string = 'redisauth'
param location string = resourceGroup().location

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


module redis '../../../../../../test/testrecipes/modules/redis-selfhost.bicep' = {
  name: '${baseName}-redis-deployment'
  params: {
    name: '${baseName}-redis'
    namespace: namespace
    application: app.name
    password: redisPassword
  }
}


resource pubsubBroker 'Applications.Dapr/pubSubBrokers@2023-10-01-preview' = {
  name: '${baseName}-dpsb'
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    type: 'pubsub.redis'
    metadata: {
      redisHost: {
        value: '${redis.outputs.host}:${redis.outputs.port}'
      }
      redisPassword: {
        secretKeyRef: {
            name: secretName
            key: 'password'
        }
      }
    }
    version: 'v1'
  }
}

resource secretstore 'Applications.Dapr/secretStores@2023-10-01-preview' = {
  name: '${baseName}-scs'
  location: location
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    type: 'secretstores.kubernetes'
    version: 'v1'
    metadata: {
      vaultName: {
        value: 'test'
      }
    }
  }
}
