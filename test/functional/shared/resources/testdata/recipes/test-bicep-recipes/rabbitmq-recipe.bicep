import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'radius-testing'
}


@description('Specifies the RabbitMQ username.')
param username string = 'guest'

@description('Specifies the RabbitMQ password.')
@secure()
param password string = 'guest'

resource rabbitmq 'apps/Deployment@v1' = {
  metadata: {
    name: 'rabbitmq-test'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'rabbitmq'
        resource: 'rabbitmq-test'
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'rabbitmq'
          resource: 'rabbitmq-test'
        }
      }
      spec: {
        containers: [
          {
            name: 'rabbitmq'
            image: 'rabbitmq:3.10'
            ports: [
              {
                containerPort: 5672
              }
            ]
            env: [
              {
                name: 'RABBIT_USERNAME'
                value: username
              }
              {
                name: 'RABBIT_PASSWORD'
                value: password
              }
            ]
          }
        ]
      }
    }
  }
}

resource svc 'core/Service@v1' = {
  metadata: {
    name: 'rabbitmq-svc'
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'rabbitmq'
      resource: 'rabbitmq-test'
    }
    ports: [
      {
        port: 5672
      }
    ]
  }
}

output result object = {
  // This workaround is needed because the deployment engine omits Kubernetes resources from its output.
  //
  // Once this gap is addressed, users won't need to do this.
  resources: [
    '/planes/kubernetes/local/namespaces/${svc.metadata.namespace}/providers/core/Service/${svc.metadata.name}'
    '/planes/kubernetes/local/namespaces/${rabbitmq.metadata.namespace}/providers/apps/Deployment/${rabbitmq.metadata.name}'
  ]
  values: {
    queue: 'queue'
    host: '${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'
    port: 5672
    username: username
  }
  secrets: {
    password: password
  }
}
