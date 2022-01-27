resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myregistry/myimage'
        env: {
          RABBITMQ: rabbitMQ.outputs.rabbitMQ.connectionString()
        }
      }
      connections: {
        messages: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitMQ.outputs.rabbitMQ.id
        }
      }
    }
  }
}

module rabbitMQ 'br:radius.azurecr.io/starters/rabbitmq:latest' = {
  name: 'rabbitmq'
  params: {
    radiusApplication: app
  }
}
