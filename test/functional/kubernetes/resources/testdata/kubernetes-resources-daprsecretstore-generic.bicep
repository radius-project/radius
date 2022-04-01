param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' string = 'radiusdev.azurecr.io/magpiego:latest' string

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-daprsecretstore-generic'

  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      connections: {
        daprsecretstore: {
          kind: 'dapr.io/SecretStore'
          source: secretstore.id
        }
      }
      container: {
        image: magpieimage
      }
    }
  }
  
  resource secretstore 'dapr.io.SecretStore@v1alpha3' = {
    name: 'secretstore'
    properties: {
      kind: 'generic'
      type: 'secretstores.kubernetes'
      metadata: {
        name: 'test'
      }
      version: 'v1'
    }
  }
}
