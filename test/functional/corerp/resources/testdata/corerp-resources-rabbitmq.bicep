import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param magpieImage string = 'radiusdev.azurecr.io/magpiego:latest'

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image for the container resource.')
param rabbitmqImage string = 'rabbitmq:3.10'

@description('Specifies the port for the container resource.')
param rabbitmqPort int = 5672

@description('Specifies the RabbitMQ username.')
param username string = 'guest'

@description('Specifies the RabbitMQ password.')
param password string = 'guest'

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
      image: magpieImage
      readinessProbe: {
        kind: 'httpGet'
        containerPort: magpiePort
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
  name: '${appPrefix}-container'
  location: location
  properties: {
    application: app.id
    container: {
      image: rabbitmqImage
      ports: {
        rabbitmq: {
          containerPort: rabbitmqPort
          provides: rabbitmqRoute.id
        }
      }
    }
  }
}

resource rabbitmqRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: '${appPrefix}-route'
  location: location
  properties: {
    application: app.id
    port: rabbitmqPort
  }
}

resource rabbitmq 'Applications.Connector/rabbitMQMessageQueues@2022-03-15-privatepreview' = {
  name: '${appPrefix}-mq'
  location: location
  properties: {
    environment: environment
    queue: 'queue'
    secrets: {
      connectionString: 'amqp://${username}:${password}@${rabbitmqRoute.properties.hostname}:${rabbitmqRoute.properties.port}'
    }
  }
}
