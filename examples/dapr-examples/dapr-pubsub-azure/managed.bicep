resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-pubsub-managed'

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
        topic: 'TOPIC_A'
        managed: true
      }
    }
  }
}
