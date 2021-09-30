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
          }
        }
      }
    }
  }
  
}
