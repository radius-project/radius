resource app 'radius.dev/Application@v1alpha3' = {
  name: 'webapp'

  resource todoapplication 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/webapptutorial-todoapp'
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
  }
  
}
