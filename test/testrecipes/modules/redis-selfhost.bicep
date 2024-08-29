extension kubernetes with {
  kubeConfig: ''
  namespace: namespace
} as kubernetes

param namespace string
param name string
param application string = ''
@secure()
param password string = ''

resource redis 'apps/Deployment@v1' = {
  metadata: {
    name: name
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'redis'
        resource: name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'redis'
          resource: name

          // Label pods with the application name so `rad run` can find the logs.
          'radapp.io/application': application == '' ? '' : application
        }
      }
      spec: {
        containers: [
          {
            // This container is the running redis instance.
            name: 'redis'
            image: 'ghcr.io/radius-project/mirror/redis:6.2'
            // Note :Using --requirepass with an empty password is
            // equivalent to setting no password
            args: [
              '--requirepass'
              password
            ]
            ports: [
              {
                containerPort: 6379
              }
            ]
          }
          {
            // This container will connect to redis and stream logs to stdout for aid in development.
            name: 'redis-monitor'
            image: 'ghcr.io/radius-project/mirror/redis:6.2'
            args: concat(
                      ['redis-cli'],
                      password != '' ? ['-a', password] : [],
                      ['-h', 'localhost', 'MONITOR']
                  )
          }
        ]
      }
    }
  }
}

resource svc 'core/Service@v1' = {
  metadata: {
    name: name
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'redis'
      resource: name
    }
    ports: [
      {
        port: 6379
      }
    ]
  }
}

output resources array = [
  '/planes/kubernetes/local/namespaces/${svc.metadata.namespace}/providers/core/Service/${svc.metadata.name}'
  '/planes/kubernetes/local/namespaces/${redis.metadata.namespace}/providers/apps/Deployment/${redis.metadata.name}'
]

output host string = '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
output port int = 6379
