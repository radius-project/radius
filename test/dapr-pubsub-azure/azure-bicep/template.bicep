resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-pubsub'

  resource nodesubscriber 'Components' = {
    name: 'nodesubscriber'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/nodesubscriber:latest'
        }
      }
      provides: [
        {
          name: 'nodesubscriber'
          kind: 'http'
          containerPort: 3000
        }
      ]
      dependsOn: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSub'
          setEnv: {
            SB_TOPIC: 'topic'
          }
        }
      ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodesubscriber'
            appPort: 3000
          }
        }
      ]
    }
  }
  
  resource pythonpublisher 'Components' = {
    name: 'nodepublisher'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/pythonpublisher:latest'
        }
      }
      dependsOn: [
        {
          name: 'pubsub'
          kind: 'dapr.io/PubSub'
          setEnv: {
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
  
  // Imagine this deployment in a separate file/repo
  resource default 'Deployments' = {
    name: 'default'
    properties: {
      components: [
        {
          componentName: pubsub.name
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
          kind: 'dapr.io/PubSub'
        }
      ]
    }
  }
}