resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //BACKEND
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'registry/container:tag'
        ports: {
          orders: {
            containerPort: 80
            provides: invoke.id
          }
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
  //BACKEND

  //ROUTE
  resource invoke 'dapr.io.InvokeRoute' = {
    name: 'invokeroute'
  }
  //ROUTE

  //FRONTEND
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env: {
          BACKEND_ID: invoke.properties.appId
        }
      }
      connections: {
        orders: {
          kind: 'dapr.io/Invoke'
          source: invoke.id
        }
      }
    }
  }
  //FRONTEND
  
}
