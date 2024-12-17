resource storageAccount 'Microsoft.Storage/storageAccounts@2021-04-01' = {
  name: 'gkmstorageaccount'
  location: 'eastus'
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {}
}
