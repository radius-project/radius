resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'cosmos-container'
  
  //SAMPLE
  resource db 'Components' = {
    name: 'db'
    kind: 'mongodb.com/Mongo@v1alpha1'
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
          binding: db.properties.bindings.mongo
          env: {
            DBCONNECTION: db.properties.bindings.mongo.connectionString
          }
        }
      ]
    }
  }

}
