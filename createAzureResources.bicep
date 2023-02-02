param location string = resourceGroup().location

@description('Specifies the SQL username.')
param adminUsername string

@secure()
param adminPassword string

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-radiustestdev'
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

  resource dbinner 'mongodbDatabases' = {
    name: 'mongodb-radiustestdev'
    properties: {
      resource: {
        id: 'mongodb-radiustestdev'
      }
      options: { 
        throughput: 400
      }
    }
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'daprns-radiustestdev'
  location: location
  tags: {
    radiustest: 'corerp-resources-dapr-pubsub-servicebus'
  }
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2021-09-01' = {
  name: 'tsaccountradiustestdev'
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    accessTier: 'Hot'
  }
  
  resource tableServices 'tableServices' = {
    name: 'default'
    
    resource table 'tables' = {
      name: 'radiustestdev'
    } 
  }
}

resource server 'Microsoft.Sql/servers@2021-02-01-preview' = {
  name: 'mssql-radiustestdev'
  location: location
  tags: {
    radiustest: 'corerp-resources-microsoft-sql'
  }
  properties: {
    administratorLogin: adminUsername
    administratorLoginPassword: adminPassword
  }

  resource db 'databases' = {
    name: 'database-radiustestdev'
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

resource redisCache 'Microsoft.Cache/redis@2020-12-01' = {
  name: 'redis-radiustestdev'
  location: location
  properties: {
    enableNonSslPort: false
    minimumTlsVersion: '1.2'
    sku: {
      family: 'C'
      capacity: 1
      name: 'Basic'
    }
  }
}

output redisCacheId string = redisCache.id
output sqlServerId string = server::db.id
output tableStorageAccId string = storageAccount::tableServices::table.id
output namespace string = namespace.id
output mongoDatabaseId string = account::dbinner.id
output documentDBId string = account.id
