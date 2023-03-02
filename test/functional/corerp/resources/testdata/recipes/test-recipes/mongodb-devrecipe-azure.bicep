@description('Specifies the location for resources.')
param location string = resourceGroup().location

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'accnt-${uniqueString(resourceGroup().name)}'
  location: location
  kind: 'MongoDB'
  tags: {
    radiustest: 'recipe-mongodb'
  }
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }

  resource dbinner 'mongodbDatabases' = {
    name: 'mdb-${uniqueString(resourceGroup().name)}'
    properties: {
      resource: {
        id: 'mongodb-1'
      }
      options: { 
        throughput: 400
      }
    }
  }
}
