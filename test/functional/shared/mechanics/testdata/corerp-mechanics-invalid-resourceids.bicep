import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

param registry string
param version string




resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-invalid-resourceids'
  location: location
  properties: {
    environment: environment
  }
}

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'invalid-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'invalid-env'
    }
    recipes: { 
      'Applications.Dapr/pubSubBrokers': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-pubsub-broker:${version}'
        }
      }
    }
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'invalid-extndr'
  properties: {
    application: app.location
    environment: env.id
    resourceProvisioning: 'manual'
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'invalid-gtwy'
  location: location
  properties: {
    application: 'not_an_id'
    routes: [
      {
        destination: ''
        path: ''
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'invalid-ctnr'
  location: location
  properties: {
    application: '/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/default/providers/applications.core/environments/env'
    container: {
      image: magpieimage
    }
  }
}
