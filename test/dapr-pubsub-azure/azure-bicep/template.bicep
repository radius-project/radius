resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-pubsub'

  resource nodesubscriber 'Components' = {
    name: 'nodesubscriber'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/dapr-pubsub-nodesubscriber:latest'
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
          image: 'vinayada/dapr-pubsub-pythonpublisher:latest'
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
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'pubsub.azure.servicebus'
        topic: 'TOPIC_A'
      }
      provides: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSubTopic'
        }
      ]
    }
  }
}
