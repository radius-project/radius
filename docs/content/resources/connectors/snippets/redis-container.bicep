//RESOURCE
import kubernetes from kubernetes

resource redisPod 'kubernetes.core/Pod@v1' = {
  metadata: {
    name: 'redis'
  }
  spec: {
    containers: [
      {
        name: 'redis:6.2'
        ports: [
          {
            containerPort: 6379
          }
        ]
      }
    ]
  }
}
//RESOURCE

resource app 'radius.dev/Application@v1alpha3' existing = {
  name: 'myapp'

  //CONNECTOR
  resource redis 'redislabs.com.RedisCache' = {
    name: 'myredis-connector'
    properties: {
      host: redisPod.spec.hostname
      port: redisPod.spec.containers[0].ports[0].containerPort
      secrets: {
        connectionString: '${redisPod.spec.hostname}.svc.cluster.local:${redisPod.spec.containers[0].ports[0].containerPort}'
        password: ''
      }
    }
  }
  //CONNECTOR

  //CONTAINER
  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myrepo/myimage'
      }
      connections: {
        inventory: {
          kind: 'redislabs.com/Redis'
          source: redis.id
        }
      }
    }
  }
  //CONTAINER

}
