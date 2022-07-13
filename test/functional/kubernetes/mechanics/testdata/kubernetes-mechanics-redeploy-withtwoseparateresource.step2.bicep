param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-mechanics-redeploy-withtwoseparateresource'

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }
}
