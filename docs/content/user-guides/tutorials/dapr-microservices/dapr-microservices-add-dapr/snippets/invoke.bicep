//SAMPLE
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-tutorial'

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

  resource invoke 'dapr.io.InvokeRoute' = {
    name: 'order-invocation'
  }
}
//SAMPLE
