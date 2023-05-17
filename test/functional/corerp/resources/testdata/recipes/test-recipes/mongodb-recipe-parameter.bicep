param documentdbName string
param mongodbName string
param location string = resourceGroup().location

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: documentdbName
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
        locationName: 'eastus'
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }

  resource dbinner 'mongodbDatabases' = {
    name: mongodbName
    properties: {
      resource: {
        id: mongodbName
      }
      options: { 
        throughput: 400
      }
    }
  }
}

output result object = {
  values: {
    host: '${account.name}.mongo.cosmos.azure.com'
    port: 443
  }
}
