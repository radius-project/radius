
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-mechanics-secrets-access'

  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: db.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          DB_CONNECTION: db.connectionString()
        }
      }
    }
  }

  resource db 'mongodb.com.MongoDBComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
}
