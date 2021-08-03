resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'kubernetes-resources-mongo'
  
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
    kind: 'mongodb.com/Mongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
