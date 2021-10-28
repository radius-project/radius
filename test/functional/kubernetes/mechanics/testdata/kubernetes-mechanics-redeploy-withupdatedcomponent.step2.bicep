resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-mechanics-redeploy-withupdatedcomponent'

  resource a 'ContainerComponent' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          'TEST': 'updated'
        }
      }
    }
  }
}
