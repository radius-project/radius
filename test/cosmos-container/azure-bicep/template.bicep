application app = {
  name: 'cosmos-container'
  
  instance webapp 'radius.dev/Container@v1alpha1' = {
    name: 'todoapp'
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

  instance db 'azure.com/CosmosDocumentDb@v1alpha1' = {
    name: 'db'
    properties: {
      config: {
        managed: true
      }
    }
  }
}