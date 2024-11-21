param location string = resourceGroup().location
param storageAccountName string

// Import the storage module
module storageModule 'storage.bicep' = {
  name: 'storageModule'
  params: {
    location: location
    storageAccountName: storageAccountName
  }
}

// Output the storage account ID
output storageAccountId string = storageModule.outputs.storageAccountId
