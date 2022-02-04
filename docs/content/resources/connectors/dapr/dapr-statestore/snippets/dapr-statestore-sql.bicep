resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dapr-statestore'

  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        pubsub: {
          kind: 'dapr.io/StateStore'
          source: statestore.id
        }
      }
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'myapp'
        }
      ]
    }
  }
  
  //SAMPLE
  resource statestore 'dapr.io.StateStore' = {
    name: 'statestore'
    properties: {
      kind: 'state.sqlserver'
      resource: sqlserver.id
    }
  }
  //SAMPLE
}

//BICEP
resource sqlserver 'Microsoft.Sql/servers@2021-05-01-preview' = {
  name: 'sqlserver${uniqueString(resourceGroup().id)}'
  location:resourceGroup().location
  properties: {
    administratorLogin: 'user${uniqueString(resourceGroup().id)}'
    administratorLoginPassword: 'p@!!${uniqueString(resourceGroup().id)}'
    version: '12.0'
    minimalTlsVersion: '1.2'
  }
}
//BICEP
