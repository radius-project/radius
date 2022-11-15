import radius as radius

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-component-name-conflict'
  location: 'global'
  properties: {
    environment: environment
  }
}

// Dapr Component #1
resource pubsub 'Applications.Link/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'dapr-component'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    mode: 'resource'
    kind: 'pubsub.azure.servicebus'
    resource: namespace.id
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'dapr-ns-${guid(resourceGroup().name)}'
  location: location
  tags: {
    radiustest: 'corerp-resources-dapr-pubsub-servicebus'
  }
}

// Dapr Component #2
resource secretstore 'Applications.Link/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'dapr-component'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    mode: 'values'
    kind: 'generic'
    type: 'secretstores.kubernetes'
    metadata: {
      vaultName: 'test'
    }
    version: 'v1'
  }
}
