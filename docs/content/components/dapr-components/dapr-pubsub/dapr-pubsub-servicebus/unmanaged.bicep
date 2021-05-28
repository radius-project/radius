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
      dependsOn: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSubTopic'
          setEnv: {
            SB_PUBSUBNAME: 'pubsubName'
            SB_TOPIC: 'topic'
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodesubscriber'
            appPort: 50051
          }
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
      dependsOn: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSubTopic'
          setEnv: {
            SB_PUBSUBNAME: 'pubsubName'
            SB_TOPIC: 'topic'
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'pythonpublisher'
          }
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
  tags: {
    radiustest: 'true'
  }

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
