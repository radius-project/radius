resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cool-app'

  resource container 'Container' = {
    name: 'container'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
}

output image string = app::container.properties.container.image
