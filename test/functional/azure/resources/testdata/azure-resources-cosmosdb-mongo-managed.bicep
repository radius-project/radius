resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-resources-cosmosdb-mongo-managed'
  
  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/magpie:latest'
        }
      }
      uses: [
        {
          binding: db.properties.bindings.mongo
          env: {
            BINDING_MONGODB_CONNECTIONSTRING: db.properties.bindings.mongo.connectionString
          }
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBMongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
