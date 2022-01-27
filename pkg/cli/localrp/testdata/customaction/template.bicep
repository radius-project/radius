resource app 'radius.dev/Application@v1alpha3' = {
  name: 'cool-app'

  resource container 'Container' = {
    name: 'container'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          DB_CONNECTION: db.connectionString()
        }
      }
    }
  }

  resource db 'mongo.com.MongoDatabase' existing = {
    name: 'db'
  }
}
