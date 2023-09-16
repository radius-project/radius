import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: namespace
}

param namespace string
param name string
param application string = ''

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
          'radius.dev/application': application
        }
      }
      spec: {
        containers: [
          {
            // This container is the running redis instance.
            name: 'redis'
            image: 'redis'
            ports: [
              {
                containerPort: 6379
              }
            ]
          }
          {
            // This container will connect to redis and stream logs to stdout for aid in development.
            name: 'redis-monitor'
            image: 'redis'
            args: [
              'redis-cli'
              '-h'
              'localhost'
              'MONITOR'
            ]
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

      // Label pods with the application name so `rad run` can find the logs.
      'radius.dev/application': application
      'radius.dev/resource': name
      'radius.dev/resource-type': 'applications.datastores-rediscaches'
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
