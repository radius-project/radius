param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' string = 'radiusdev.azurecr.io/magpiego:latest' string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-mechanics-redeploy-withanotherresource'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: magpieimage
      }
    }
  }
}
