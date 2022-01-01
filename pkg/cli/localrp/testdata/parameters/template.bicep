param image string 

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cool-app'

  resource container 'Container' = {
    name: 'container'
    properties: {
      container: {
        image: image
      }
    }
  }
}
