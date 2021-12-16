resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-dapr-pubsub-servicebus-unmanaged'

  resource publisher 'Container' = {
    name: 'publisher'
    properties: {
      connections: {
        daprpubsub: {
          kind: 'dapr.io/PubSubTopic'
          source: pubsub.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          BINDING_DAPRPUBSUB_NAME: pubsub.properties.pubSubName
          BINDING_DAPRPUBSUB_TOPIC: pubsub.properties.topic
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'publisher'
          appPort: 3000
        }
      ]
    }
  }
  
  resource pubsub 'dapr.io.PubSubTopic' = {
    name: 'pubsub'
    properties: {
      kind: 'pubsub.azure.servicebus'
      resource: namespace::topic.id
    }
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'ns-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  tags: {
    radiustest: 'azure-resources-dapr-pubsub-servicebus-unmanaged'
  }

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
