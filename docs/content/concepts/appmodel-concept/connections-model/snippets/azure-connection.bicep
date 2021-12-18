resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-storage-app'

  resource store 'Container' = {
    name: 'storage-service'
    properties: {
      container: {
        image: 'registry/container:tag'
      }
      connections: {
        storageresource: {
          kind:'azure'
          source: storageAccount.id
          role: [
            'Reader and Data Access'
            'Storage Blob Data Contributor'
          ]
        }
      }
    }
  }
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2021-04-01' = {
  name: 'StorageAccount-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'StorageV2'
  sku: {
    name: 'Standard_LRS'
  }
}
