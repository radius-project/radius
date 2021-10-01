resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-cosmosdb-mongo-managed'

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

  resource db 'azure.com.CosmosDBMongoComponent' = {
    name: 'db'
    properties: {
      managed: true
    }
  }
}
