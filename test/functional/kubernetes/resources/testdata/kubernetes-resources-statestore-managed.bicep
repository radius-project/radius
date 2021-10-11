resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-statestore-managed'

  resource daprroute 'dapr.io.DaprHttpRoute' ={
    name: 'daprroute'
    properties: {
      appId: 'receiver'
    }
  }
  
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
          provides: daprroute.id
          appPort: 3000
          appId: 'receiver'
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
