resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq-managed'

  resource webapp 'Container' = {
    name: 'todoapprabbitmq'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          BINDING_RABBITMQ_CONNECTIONSTRING: rabbitmqMessageQueue.connectionString()
        }
      }
      connections: {
        rabbitmq: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmqMessageQueue.id
        }
      }
    }
  }
  resource rabbitmqMessageQueue 'rabbitmq.com.MessageQueue' existing = {
    name: 'queue'
  }
}

module rabbitmq 'br:radius.azurecr.io/starters/rabbitmq:latest' = {
  name: 'rabbitmq-module'
  params: {
    radiusApplication: app
    brokerName: 'broker'
    queueName: 'queue'
  }
}
