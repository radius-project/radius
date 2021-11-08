resource app 'radius.dev/Application@v1alpha3' = {
  name: 'todo'

  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      resource: account::db.id
    }
  }
}

resource account 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'account-${guid(resourceGroup().name)}'
  location: resourceGroup().location
  kind: 'MongoDB'
  tags: {
    radiustest: 'azure-resources-mongodb-unmanaged'
  }
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: resourceGroup().location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }

  resource db 'mongodbDatabases' = {
    name: 'mydb'
    properties: {
      resource: {
        id: 'mydb'
      }
      options: { 
        throughput: 400
      }
    }
  }
}

output database_name string = app::db.name
