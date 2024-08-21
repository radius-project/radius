param location string = resourceGroup().location

@description('Specifies the SQL username.')
param adminUsername string

@secure()
param adminPassword string

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

resource server 'Microsoft.Sql/servers@2021-02-01-preview' = {
  name: 'mssql-radiustest'
  location: location
  tags: {
    radiustest: 'corerp-resources-microsoft-sql'
  }
  properties: {
    administratorLogin: adminUsername
    administratorLoginPassword: adminPassword
  }

  resource db 'databases' = {
    name: 'database-radiustest'
    location: location
  }

  resource firewall 'firewallRules' = {
    name: 'allow'
    properties: {
      startIpAddress: '0.0.0.0'
      endIpAddress: '0.0.0.0'
    }
  }
}

output sqlServerId string = server::db.id
output cosmosMongoAccountID string = account.id
