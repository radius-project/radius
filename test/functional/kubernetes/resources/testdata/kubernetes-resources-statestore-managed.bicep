resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-statestore-managed'

  resource receiverapplication 'ContainerComponent' = {
    name: 'sender'
    properties: {
      connections: {
        daprstatestore: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'receiver'
          appPort: 3000
        }
      ]
    }
  }

  resource statestore 'dapr.io.StateStoreComponent' = {
    name: 'statestore'
    properties: {
      kind: 'any'
      managed: true
    }
  }
}
