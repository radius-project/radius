resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'my-application'

  resource frontend 'ContainerComponent' = {
    name: 'frontend-service'
    properties: {
      container: {
        image: 'nginx:latest'
      }
    }
  }
}
