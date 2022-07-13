import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-statestore-tablestorage'
  location: location
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'myapp'
  location: location
  properties: {
    application: app.id
    connections: {
      daprstatestore: {
        //kind: 'dapr.io/StateStore'
        source: statestore.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
  }
}

resource account 'Microsoft.Storage/storageAccounts@2019-06-01' = {
  name: 'dapr${uniqueString(resourceGroup().id, deployment().name)}'
  location: location
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

resource statestore 'Applications.Connector/daprStateStores@2022-03-15-privatepreview' = {
  name: 'mystore'
  location: location
  properties: {
    environment: environment
    kind: 'state.azure.tablestorage'
    resource: account::tableServices::table.id
  }
}
