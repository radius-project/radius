param location string = resourceGroup().location

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-radiustest'
  location: location
  kind: 'MongoDB'
  tags: {
    radiustest: 'corerp-resources-mongodb'
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
}

output cosmosMongoAccountID string = account.id
