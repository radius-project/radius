param context object 
param location string = resourceGroup().location
param rg string = resourceGroup().name

resource account 'Microsoft.DocumentDB/databaseAccounts@2021-10-15-preview' = {
  name: 'account-${rg}'
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

  resource dbinner 'mongodbDatabases@2021-10-15-preview' = {
    name: '${context.resource.name}-${rg}'
    properties: {
      resource: {
        id: '${context.resource.name}-${rg}'
      }
      options: { 
        throughput: 400
      }
    }
  }
}

output result object = {
  values: {
    host: 'account-${rg}.mongo.cosmos.azure.com'
    port: 10255
    database: account::dbinner.name
    username: account.name
  }
  secrets: {
    connectionString: account.listConnectionStrings().connectionStrings[0].connectionString
    password: account.listKeys().primaryMasterKey
  }
}
