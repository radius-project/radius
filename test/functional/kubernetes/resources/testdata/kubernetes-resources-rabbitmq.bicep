param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param port int = 5672
param username string = 'guest'
param password string = 'guest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq'

  resource webapp 'Container' = {
    name: 'webapp'
    properties: {
      container: {
        image: magpieimage
        readinessProbe: {
          kind: 'httpGet'
          containerPort: 3000
          path: '/healthz'
        }
      }
      connections: {
        rabbitmq: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
        }
      }
    }
  }

  resource rabbitmqContainer 'Container' = {
    name: 'rabbitmq-container'
    properties: {
      container: {
        image: 'rabbitmq:3.10'
        ports: {
          rabbitmq: {
            containerPort: port
            provides: rabbitmqRoute.id
          }
        }
      }
    }
  }

  resource rabbitmqRoute 'HttpRoute' = {
    name: 'rabbitmq-route'
    properties: {
      port: port
    }
  }

  resource rabbitmq 'rabbitmq.com.MessageQueue' = {
    name: 'rabbitmq'
    properties: {
      queue: 'queue'
      secrets: {
        connectionString: 'amqp://${username}:${password}@${rabbitmqRoute.properties.host}:${rabbitmqRoute.properties.port}'
      }
    }
  }
}
