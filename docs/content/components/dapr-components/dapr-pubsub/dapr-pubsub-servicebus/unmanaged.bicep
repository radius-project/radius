resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-pubsub-unmanaged'

  resource nodesubscriber 'Components' = {
    name: 'nodesubscriber'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/dapr-pubsub-nodesubscriber:latest'
        }
      }
      uses: [
        {
          binding: pubsub.properties.bindings.default
          env: {
            SB_PUBSUBNAME: pubsub.properties.bindings.default.pubSubName
            SB_TOPIC: pubsub.properties.bindings.default.topic
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'nodesubscriber'
          appPort: 50051
        }
      ]
    }
  }
  
  resource pythonpublisher 'Components' = {
    name: 'pythonpublisher'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/dapr-pubsub-pythonpublisher:latest'
        }
      }
      uses: [
        {
          binding: pubsub.properties.bindings.default
          env: {
            SB_PUBSUBNAME: pubsub.properties.bindings.default.pubSubName
            SB_TOPIC: pubsub.properties.bindings.default.topic
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'pythonpublisher'
        }
      ]
    }
  }
  
  resource pubsub 'Components' = {
    name: 'pubsub'
    kind: 'dapr.io/PubSubTopic@v1alpha1'
    properties: {
      config: {
        kind: 'pubsub.azure.servicebus'
        resource: namespace::topic.id
      }
    }
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'ns-${guid(resourceGroup().name)}'
  location: resourceGroup().location

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
