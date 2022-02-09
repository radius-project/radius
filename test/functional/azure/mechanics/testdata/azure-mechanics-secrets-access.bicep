
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-mechanics-secrets-access'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      connections: {
        mongodb: {
          kind: 'mongo.com/MongoDB'
          source: mongoDatabase.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          DB_CONNECTION: mongoDatabase.connectionString()
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource mongoDatabase 'mongo.com.MongoDatabase' existing = {
    name: 'db'
  }
}

module db 'br:radius.azurecr.io/starters/mongo-azure:latest' = {
  name: 'db-module'
  params: {
    radiusApplication: app
    dbName: 'db'
  }
}
