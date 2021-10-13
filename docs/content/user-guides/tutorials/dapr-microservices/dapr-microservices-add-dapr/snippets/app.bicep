resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

  //SAMPLE
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      //RUN
      container: {
        image: 'radius.azurecr.io/daprtutorial-backend'
        ports: {
          orders: {
            containerPort: 3000
            provides: daprBackend.id
          }
        }
      }
      //RUN
      connections: {
        orders: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      //TRAITS
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appPort: 3000
          appId: 'backend'
          provides: daprBackend.id
        }
      ]
      //TRAITS
    }
  }

  //STATESTORE
  resource statestore 'dapr.io.StateStoreComponent' = {
    name: 'statestore'
    properties: {
      kind: 'any'
      managed: true
    }
  }
  //STATESTORE
  //SAMPLE

  resource daprBackend 'dapr.io.DaprHttpRoute' = {
    name: 'dapr-backend'
    properties: {
      appId: 'backend'
    }
  }

}
