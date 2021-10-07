resource app 'radius.dev/Application@v1alpha3' = {
  name: 'redis-container'

  //SAMPLE
  resource redis 'redislabs.com.RedisComponent' = {
    name: 'redis'
    properties: {
      managed: true
    }
  }
  //SAMPLE

  resource webapp 'ContainerComponent' = {
    name: 'todoapp'
    properties: {
      //HIDE
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          CONNECTIONSTRING: '${redis.properties.host}:${redis.properties.port},password=${redis.password()},ssl=True,abortConnect=False'
        }
      }
      //HIDE
      connections: {
        redis: {
          kind: 'redislabs.com/Redis'
          source: redis.id
        }
      }
    }
  }

}
