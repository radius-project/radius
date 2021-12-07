//COSMOS
resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' existing = {
  name: 'myaccount'
  
  resource db 'mongodbDatabases' existing = {
    name: 'mydb'
  }
}
//COSMOS

resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'my-application'

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo-db'
    properties: {
      resource: cosmos::db.id
    }
  }
}
