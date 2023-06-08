import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
}

param context object
@description('Specifies the RabbitMQ username.')
param username string = 'guest'

@description('Specifies the RabbitMQ password.')
@secure()
param password string

resource rabbitmq 'apps/Deployment@v1' = {
  metadata: {
    name: 'rabbitmq-${uniqueString(context.resource.id)}'
  }
  spec: {
    selector: {
      matchLabels: {
        app: 'rabbitmq'
        resource: context.resource.name
      }
    }
    template: {
      metadata: {
        labels: {
          app: 'rabbitmq'
          resource: context.resource.name
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
    name: 'rabbitmq-${uniqueString(context.resource.id)}'
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'rabbitmq'
      resource: context.resource.name
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
  }
  secrets: {
    connectionString: 'amqp://${username}:${password}@${'${svc.metadata.name}.${svc.metadata.namespace}.svc.cluster.local'}:5672'
  }
}
