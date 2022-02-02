resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-statestore'
  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        pubsub: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'myapp'
        }
      ]
    }
  }

  //SAMPLE
  resource statestore 'dapr.io.StateStore' = {
    name: 'statestore'
    properties: {
      kind: 'state.redis'
      managed: true
    }
  }
  //SAMPLE
}
