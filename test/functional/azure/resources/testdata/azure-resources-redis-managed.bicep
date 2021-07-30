resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-resources-redis-managed'
  
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
          binding: redis.properties.bindings.redis
          env: {
            BINDING_REDIS_CONNECTIONSTRING: redis.properties.bindings.redis.connectionString
          }
        }
      ]
    }
  }

  resource redis 'Components' = {
    name: 'redis'
    kind: 'redislabs.com/Redis@v1alpha1'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
