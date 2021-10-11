resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-mechanics-redeploy-withanothercomponent'

  resource a 'ContainerComponent' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
}
