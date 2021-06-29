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
      uses: [
        {
          binding: db.properties.bindings.sql
          env: {
            DBCONNECTION: db.properties.bindings.sql.connectionString
          }
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBSQL@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
