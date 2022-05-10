param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
resource account 'Microsoft.Storage/storageAccounts@2019-06-01' = {
  name: 'dapr${uniqueString(resourceGroup().id, deployment().name)}'
  location: resourceGroup().location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    accessTier: 'Hot'
  }
  
  resource tableServices 'tableServices' = {
    name: 'default'
    
    resource table 'tables' = {
      name: 'mytable'
    }
    
  }
  
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-dapr-statestore-tablestorage'

  resource myapp 'Container' = {
    name: 'myapp'
    properties: {
      connections: {
        daprstatestore: {
          kind: 'dapr.io/StateStore'
          source: stateStore.id
        }
      }
      container: {
        image: magpieimage
     }
    }
  }

  resource stateStore 'dapr.io.StateStore' = {
    name: 'mystore'
    properties: {
      kind: 'state.azure.tablestorage'
      resource: account::tableServices::table.id
    }
  }

}
