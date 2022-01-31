resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq-managed'

  resource webapp 'Container' = {
    name: 'todoapprabbitmq'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          BINDING_RABBITMQ_CONNECTIONSTRING: rabbitmq.outputs.rabbitMQ.connectionString()
        }
      }
      connections: {
        rabbitmq: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.outputs.rabbitMQ.id
        }
      }
    }
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
