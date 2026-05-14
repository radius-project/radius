param location string = resourceGroup().location

@description('Name of the Cosmos DB (MongoDB) account. Cosmos account names are globally unique, so callers running in parallel or recreating the deployment shortly after a delete must override this with a unique value.')
param cosmosAccountName string = 'account-radiustest'

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: cosmosAccountName
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
