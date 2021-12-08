resource app 'radius.dev/Application@v1alpha3' = {
  name: 'webapp'

  resource todoRoute 'HttpRoute' = {
    name: 'todo-route'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }

  resource todoapplication 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/webapptutorial-todoapp'
        ports: {
          web: {
            containerPort: 3000
            provides: todoRoute.id
          }
        }
      }
    }
  }
  
}
