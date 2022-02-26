resource app 'radius.dev/Application@v1alpha3' = {
  name: 'testsecretstore'

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
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }
  
  resource secretstore 'dapr.io.SecretStore@v1alpha3' = {
    name: 'secretstore-generic'
    properties: {
      kind: 'generic'
      type: 'secretstores.azure.keyvault'
      metadata: {
        foo: 'bar'
      }
      version: 'v1'
      
    }
  }
}
    
   
    
    
    
