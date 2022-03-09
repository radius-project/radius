resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-mechanics-redeploy-withtwoseparateresource'

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpiego:latest'
      }
    }
  }
}
