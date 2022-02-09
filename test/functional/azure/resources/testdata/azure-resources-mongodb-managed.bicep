// NOTE: This file is here for manual testing purposes.
// we intentionally omit automated tests for some of the Azure resource
// types because it would massively bloat our runs.

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-mongodb-managed'

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
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
  resource mongoDatabase 'mongo.com.MongoDatabase' existing = {
    name: 'starterdb'
  }
}

module db 'br:radius.azurecr.io/starters/mongo-azure:latest' = {
  name: 'db-module'
  params: {
    radiusApplication: app
    dbName: 'starterdb'
  }
}
