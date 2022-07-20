import radius as radius

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-pubsub-servicebus'
  location: location
  properties: {
    environment: environment
  }
}

resource pubsub 'Applications.Connector/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'sb-pubsub'
  location: location
  properties: {
    environment: environment
    application: app.id
    kind: 'pubsub.azure.servicebus'
    resource: namespace::topic.id
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'ns-kachawla'
  location: location
  tags: {
    radiustest: 'corerp-resources-dapr-pubsub-servicebus'
  }

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
