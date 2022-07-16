import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-pubsub-servicebus'
  location: location
  properties: {
    environment: environment
  }
}

resource publisher 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'publisher'
  location: location
  properties: {
    application: app.id
    connections: {
      daprpubsub: {
        source: pubsub.id
      }
    }
    container: {
      image: magpieimage
      env: {
        BINDING_DAPRPUBSUB_NAME: pubsub.name
        BINDING_DAPRPUBSUB_TOPIC: pubsub.properties.topic
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'publisher'
        appPort: 3000
      }
    ]
  }
}

resource pubsub 'Applications.Connector/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'pubsub'
  location: location
  properties: {
    environment: environment
    application: app.id
    kind: 'pubsub.azure.servicebus'
    resource: namespace::topic.id
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'ns-${guid(resourceGroup().name)}'
  location: location
  tags: {
    radiustest: 'corerp-resources-dapr-pubsub-servicebus'
  }

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
