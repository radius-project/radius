param location string = resourceGroup().location

@description('Specifies the SQL username.')
param adminUsername string

@description('Specifies the SQL password.')
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

  resource dbinner 'mongodbDatabases' = {
    name: 'mongodb-radiustest'
    properties: {
      resource: {
        id: 'mongodb-radiustest'
      }
      options: { 
        throughput: 400
      }
    }
  }
}

resource namespace 'Microsoft.ServiceBus/namespaces@2017-04-01' = {
  name: 'daprns-radiustest'
  location: location
  tags: {
    radiustest: 'corerp-resources-dapr-pubsub-servicebus'
  }
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2021-09-01' = {
  name: 'tsaccountradiustest'
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
      name: 'radiustest'
    } 
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
