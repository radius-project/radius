resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container'
  
  //SAMPLE
  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDBSQL@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
  //SAMPLE

  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
        }
      }
      //HIDE
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

}
