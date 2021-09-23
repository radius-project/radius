resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'redis-container'
  
  //SAMPLE
  resource redis 'Components' = {
    name: 'redis'
    kind: 'redislabs.com/Redis@v1alpha1'
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
          image: 'radius.azurecr.io/magpie:latest'
        }
      }
      //HIDE
      uses: [
        {
          binding: redis.properties.bindings.redis
          env: {
            HOST: redis.properties.bindings.redis.host
          }
        }
      ]
    }
  }

}
