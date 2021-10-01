resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-cli'

  resource a 'ContainerComponent' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource b 'ContainerComponent' = {
    name: 'b'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
}
