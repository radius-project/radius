resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  //CONTAINER
  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //RUN
      run: {
        container: {
          image: 'radius.azurecr.io/webapptutorial-todoapp'
        }
      }
      //RUN
      uses: [
        {
          binding: db.properties.bindings.mongo
          env: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      ]
      //BINDINGS
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
      //BINDINGS
    }
  }
  //CONTAINER

  //MONGO
  resource db 'Components' = {
    name: 'db'
    kind: 'mongodb.com/Mongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
  //MONGO
}
