import kubernetes from kubernetes

param  magpieimage string

resource rabbitmqService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'rabbitmq-svc'
    namespace: 'default'
  }
}

resource rabbitmqSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'rabbitmq-pw'
    namespace: 'default'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq'

  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      container: {
        image: magpieimage
      }
      connections: {
        rabbitmq: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
        }
      }
    }
  }

  resource rabbitmq 'rabbitmq.com.MessageQueue' = {
    name: 'rabbitmq'
    properties: {
			queue: 'queue'
      secrets: {
        connectionString: 'amqp://admin:${base64ToString(rabbitmqSecret.data['rabbitmq-password'])}@${rabbitmqService.metadata.name}.${rabbitmqService.metadata.namespace}.svc.cluster.local:${rabbitmqService.spec.ports[0].port}'
      }
    }
  }
}
