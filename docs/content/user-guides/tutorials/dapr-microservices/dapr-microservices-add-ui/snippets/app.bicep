//SAMPLE
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

  //FRONTEND
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'radius.azurecr.io/daprtutorial-frontend'
        ports:{
          ui: {
            containerPort: 80
          }
        }
      }
      connections: {
        orders: {
          kind: 'dapr.io/Invoke'
          source: invoke.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'frontend'
        }
      ]
    }
  }
  //FRONTEND

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/daprtutorial-backend'
        ports: {
          orders: {
            containerPort: 3000
            provides: invoke.id
          }
        }
      }
      connections: {
        orders: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'backend'
          appPort: 3000
        }
      ]
    }
  }

  resource invoke 'dapr.io.InvokeRoute' = {
    name: 'order-invocation'
  }

  resource statestore 'dapr.io.StateStoreComponent' = {
    name: 'statestore'
    properties: {
      kind: 'any'
      managed: true
    }
  }
}
