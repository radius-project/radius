import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param image string = 'radiusdev.azurecr.io/magpiego:latest'

@description('Specifies the port for the container resource.')
param port int = 5672

@description('Specifies the RabbitMQ username.')
param username string = 'guest'

@description('Specifies the RabbitMQ password.')
param password string = 'guest'

@description('Specifies the environment for resources.')
param environment string = 'test'

var appPrefix = 'corerp-resources-rabbitmq'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: '${appPrefix}-app'
  location: location
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: '${appPrefix}-webapp'
  location: location
  properties: {
    application: app.id
    container: {
      image: image
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    connections: {
      rabbitmq: {
        source: rabbitmq.id
      }
    }
  }
}

resource rabbitmqContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: '${appPrefix}-rabbitmq-container'
  location: location
  properties: {
    application: app.id
    container: {
      image: image
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    connections: {
      rabbitmq: {
        source: rabbitmq.id
      }
    }
  }
}

resource rabbitmqRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: '${appPrefix}-rabbitmq-route'
  location: location
  properties: {
    application: app.id
    port: port
  }
}

resource rabbitmq 'Applications.Connector/rabbitMQMessageQueues@2022-03-15-privatepreview' = {
  name: '${appPrefix}-rabbitmq'
  location: location
  properties: {
    environment: environment
    queue: 'queue'
    secrets: {
      connectionString: 'amqp://${username}:${password}@${rabbitmqRoute.properties.hostname}:${rabbitmqRoute.properties.port}'
    }
  }
}
