resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-mongo'
  
  resource webapp 'Container' = {
    name: 'todomongo'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'

        // This is here so we make sure to exercise the 'programmatic secrets' code path.
        env: {
          DB_CONNECTION: mongoDatabase.connectionString()
        }
      }
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongoDatabase.id
        }
      }
    }
  }
  resource mongoDatabase 'mongo.com.MongoDatabase' existing = {
    name: 'mongodb'
  }
}

module db 'br:radius.azurecr.io/starters/mongo:latest' = {
  name: 'db'
  params: {
    radiusApplication: app
    dbName: 'mongodb'
  }
}
