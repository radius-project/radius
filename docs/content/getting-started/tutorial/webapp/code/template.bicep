resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp'

  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
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
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
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

  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
