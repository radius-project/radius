import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image for the container resource.')
param magpieImage string

@description('Specifies the port for the container resource.')
param magpiePort int = 3000

@description('Specifies the RabbitMQ password.')
@secure()
param password string

param registry string

param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-rabbitmq-recipe-env-old'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-rabbitmq-recipe-env-old'
    }
    recipes: {
      'Applications.Link/rabbitMQMessageQueues': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/rabbitmq-recipe:${version}'
          parameters: {
            password: password
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-rabbitmq-recipe-old'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-rabbitmq-default-recipe-app-old'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'rmq-recipe-app-ctnr-old'
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
  name: 'rmq-recipe-resource-old'
  location: location
  properties: {
    application: app.id
    environment: env.id
  }
}
