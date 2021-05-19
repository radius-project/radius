resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container'
  
  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
        }
      }
      dependsOn: [
        {
          name: 'db'
          kind: 'mongodb.com/Mongo'
          setEnv: {
            DB_CONNECTION: 'connectionString'
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
