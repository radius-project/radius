param context object 
param location string = resourceGroup().location
param rg string = resourceGroup().name

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
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

  resource dbinner 'mongodbDatabases' = {
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
