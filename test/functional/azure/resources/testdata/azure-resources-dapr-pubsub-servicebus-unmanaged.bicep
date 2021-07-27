resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-resources-dapr-pubsub-servicebus-unmanaged'

  resource publisher 'Components' = {
    name: 'publisher'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/magpie:latest'
        }
      }
      uses: [
        {
          binding: pubsub.properties.bindings.default
          env: {
            BINDING_DAPRPUBSUB_NAME: pubsub.properties.bindings.default.pubSubName
            BINDING_DAPRPUBSUB_TOPIC: pubsub.properties.bindings.default.topic
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'publisher'
          appPort: 3000
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
    radiustest: 'azure-resources-dapr-pubsub-servicebus-unmanaged'
  }

  resource topic 'topics' = {
    name: 'TOPIC_A'
  }
}
