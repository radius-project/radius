import radius as radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'daprrp-rs-component-name-conflict'
  location: 'global'
  properties: {
    environment: environment
  }
}

// Dapr Component #1
resource pubsub 'Applications.Dapr/pubSubBrokers@2023-10-01-preview' = {
  name: 'dapr-component'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    type: 'pubsub.azure.servicebus'
    version: 'v1'
    metadata: {
      name: 'test'
    }
  }
}

// Dapr Component #2
resource secretstore 'Applications.Dapr/secretStores@2023-10-01-preview' = {
  name: 'dapr-component'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    type: 'secretstores.kubernetes'
    metadata: {
      vaultName: 'test'
    }
    version: 'v1'
  }
}
