resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-pubsub'

  resource nodesubscriber 'ContainerComponent' = {
    name: 'nodesubscriber'
    properties: {
      container: {
        image: 'radiusteam/dapr-pubsub-nodesubscriber:latest'
        env: {
          SB_PUBSUBNAME: pubsub.properties.pubSubName
          SB_TOPIC: pubsub.properties.topic
        }
      }
      connections: {
        pubsub: {
          kind: 'dapr.io/PubSubTopic'
          source: pubsub.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'nodesubscriber'
          appPort: 50051
        }
      ]
    }
  }

  resource pythonpublisher 'ContainerComponent' = {
    name: 'pythonpublisher'
    properties: {
      container: {
        image: 'radiusteam/dapr-pubsub-pythonpublisher:latest'
        env: {
          SB_PUBSUBNAME: pubsub.properties.pubSubName
          SB_TOPIC: pubsub.properties.topic
        }
      }
      connections: {
        pubsub: {
          kind: 'dapr.io/PubSubTopic'
          source: pubsub.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'pythonpublisher'
        }
      ]
    }
  }
  
  //SAMPLE
  resource pubsub 'dapr.io.PubSubTopicComponent' = {
    name: 'pubsub'
    properties: {
      kind: 'pubsub.azure.servicebus'
      resource: namespace::topic.id
    }
  }
  //SAMPLE
}

//BICEP
resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'ns-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
//BICEP
