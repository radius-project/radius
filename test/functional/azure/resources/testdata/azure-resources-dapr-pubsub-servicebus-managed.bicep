resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-dapr-pubsub-servicebus-managed'

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
      topic: 'TOPIC_A'
      managed: true
    }
  }
}
