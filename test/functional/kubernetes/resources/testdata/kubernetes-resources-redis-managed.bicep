resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'kubernetes-resources-redis-managed'
  
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
            BINDING_REDIS_HOST: redis.properties.bindings.redis.host
            BINDING_REDIS_PORT: redis.properties.bindings.redis.port
            BINDING_REDIS_PASSWORD: redis.properties.bindings.redis.primaryKey
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
