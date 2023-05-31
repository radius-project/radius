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

param registry string 

param version string

param scope string = resourceGroup().id

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-rabbitmq-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-rabbitmq-recipe-env'
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Link/rabbitMQMessageQueues':{
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/corerp/recipes/rabbitmq-recipe:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-rabbitmq-default-recipe'
  location: 'global'
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-rabbitmq-default-recipe-app'
      }
    ]
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

resource rabbitmq 'Applications.Link/rabbitMQMessageQueues@2022-03-15-privatepreview' = {
  name: 'rmq-rmq'
  location: location
  properties: {
    application: app.id
    environment: environment
  }
}
