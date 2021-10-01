// NOTE: This file is here for manual testing purposes.
// we intentionally omit automated tests for some of the Azure resource
// types because it would massively bloat our runs.

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-mongodb-managed'

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
