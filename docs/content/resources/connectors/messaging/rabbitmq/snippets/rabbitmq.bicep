@description('Admin username for the RabbitMQ broker. Default is "guest"')
param username string = 'guest'

@description('Admin password for the RabbitMQ broker')
@secure()
param password string = newGuid()

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq-managed'
  
  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      //HIDE
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          BINDING_RABBITMQ_CONNECTIONSTRING: rabbitmq.connectionString()
        }
      }
      //HIDE
      connections: {
        messages: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
        }
      }
    }
  }

  resource rabbitmqContainer 'Container' = {
    name: 'rmq-container'
    properties: {
      container: {
        image: 'rabbitmq:3.9'
        ports: {
          rabbitmq: {
            containerPort: 5672
            provides: rmqContainer.id
          }
        }
      }
    }
  }

  resource rmqContainer 'HttpRoute' = {
    name: 'redis-route'
    properties: {
      port: 5672
    }
  }

  //SAMPLE
  resource rabbitmq 'rabbitmq.com.MessageQueue' = {
    name: 'rabbitmq'
    properties: {
      queue: 'radius-queue'
      secrets: {
        connectionString: 'amqp://${username}:${password}@${rmqContainer.properties.host}:${rmqContainer.properties.port}'
      }
    }
  }
  //SAMPLE
}
