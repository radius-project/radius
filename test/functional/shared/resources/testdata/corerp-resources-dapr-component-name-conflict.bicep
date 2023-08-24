import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dcnc-old'
  location: 'global'
  properties: {
    environment: environment
  }
}

// Dapr Component #1
resource pubsub 'Applications.Link/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'dapr-component-old'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    type: 'pubsub.azure.servicebus'
    version: 'v1'
    metadata: {
      name: 'example'
    }
  }
}

// Dapr Component #2
resource secretstore 'Applications.Link/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'dapr-component-old'
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
