resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'webapp-kv'

  resource todoapplication 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/tutorial-todoapp'
        }
      }
      dependsOn: [
        {
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'kvuri'
          }
        }
        {
          kind: 'mongodb.com/Mongo'
          name: 'db'
          setSecret: {
            DB_CONNECTION_SECRET: 'connectionString'
          }
        }
      ]
      provides: [
        {
          kind: 'http'
          name: 'web'
          containerPort: 3000
        }
      ]
    }
  }

  resource db 'Components' = {
    name: 'db'
    kind: 'azure.com/CosmosDocumentDb@v1alpha1'
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
