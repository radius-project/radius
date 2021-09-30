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
            provides: invoke.id
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
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
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

  resource invoke 'dapr.io.InvokeRoute' = {
    name: 'order-invocation'
  }

}

