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
    name: '${context.resource.name}-db'
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
    host: account.properties.documentEndpoint
    port: 443
    database: account::dbinner.name
  }
  secrets: {
    connectionString: 'mongodb://${account.properties.documentEndpoint}:443/${context.resource.name}-db'
  }
}
