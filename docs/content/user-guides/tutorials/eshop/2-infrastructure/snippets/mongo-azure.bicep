//REST
//REST

resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2021-06-15' existing = {
  name: 'eshop'

  resource db 'mongodbDatabases' existing = {
    name: 'db'
  }
}

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource mongo 'mongodb.com.MongoDBComponent' = {
    name: 'mongo'
    properties: {
      resource: cosmos::db.id
    }
  }
}
