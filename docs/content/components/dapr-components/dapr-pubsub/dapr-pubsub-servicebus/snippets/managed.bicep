resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-pubsub'

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
        topic: 'TOPIC_A'
        managed: true
      }
    }
  }
}
