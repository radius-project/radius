resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-mechanics-redeploy-withtwoseparatecomponent'

  resource a 'ContainerComponent' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
}
