@description('Specifies the location for resources.')
param location string = resourceGroup().location
resource account 'Microsoft.Storage/storageAccounts@2021-09-01' = {
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
