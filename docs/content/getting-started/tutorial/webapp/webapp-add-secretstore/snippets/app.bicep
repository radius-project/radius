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
          binding: kv.properties.bindings.default
          env: {
            KV_URI: kv.properties.bindings.default.uri
          }
        }
        {
          binding: db.properties.bindings.mongo
          secrets: {
            store: kv.properties.bindings.default
            keys: {
              DBCONNECTION: db.properties.bindings.mongo.connectionString
            }
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

  //DATABASE
  resource db 'Components' = {
    name: 'db'
    kind: 'mongodb.com/Mongo@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
  //DATEBASE

  //KEYVAULT
  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
        config: {
            managed: true
        }
    }
  }
  //KEYVAULT
}
