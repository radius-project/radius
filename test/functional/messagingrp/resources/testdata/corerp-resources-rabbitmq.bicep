import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param magpieImage string

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image for the RabbitMQ container resource.')
param rabbitmqImage string = 'rabbitmq:3.10'

@description('Specifies the port for the container resource.')
param rabbitmqPort int = 5672

@description('Specifies the RabbitMQ username.')
param username string = 'guest'

@description('Specifies the RabbitMQ password.')
@secure()
param password string 

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-rabbitmq'
  location: location
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'rmq-app-ctnr'
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
  name: 'rmq-ctnr'
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
  name: 'rmq-rte'
  location: location
  properties: {
    application: app.id
    port: rabbitmqPort
  }
}

resource rabbitmq 'Applications.Messaging/rabbitMQMessageQueues@2022-03-15-privatepreview' = {
  name: 'rmq-rmq'
  location: location
  properties: {
    application: app.id
    environment: environment
    resourceProvisioning: 'manual'
    queue: 'queue'
    secrets: {
      connectionString: 'amqp://${username}:${password}@${rabbitmqRoute.properties.hostname}:${rabbitmqRoute.properties.port}'
    }
  }
}
