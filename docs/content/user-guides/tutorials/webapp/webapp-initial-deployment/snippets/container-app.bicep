resource app 'radius.dev/Application@v1alpha3' = {
  name: 'webapp'

  resource route 'HttpRoute' = {
    name: 'route'
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
            provides: route.id
          }
        }
      }
    }
  }
  
}
