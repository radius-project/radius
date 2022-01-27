
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

  //MONGO
  resource mongo 'mongo.com.MongoDatabase' = {
    name: 'mongo-db'
    properties: {
      resource: cosmos::db.id
    }
  }
  //MONGO

  resource frontend 'Container' = {
    name: 'frontend-service'
    properties: {
      //CONTAINER
      container: {
        image: 'nginx:latest'
      }
      //CONTAINER
      connections: {
        db: {
          kind: 'mongo.com/MongoDB'
          source: mongo.id
        }
      }
    }
  }
}
