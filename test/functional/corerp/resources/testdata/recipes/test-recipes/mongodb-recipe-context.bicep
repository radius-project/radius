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
    host: split(split(account.properties.documentEndpoint, '://')[1], ':')[0]
    port: 443
    database: '${context.resource.name}-${rg}'
  }
  secrets: {
    connectionString: 'mongodb://${split(split(account.properties.documentEndpoint, '://')[1], ':')[0]}:443/${context.resource.name}-${rg}'
  }
}
