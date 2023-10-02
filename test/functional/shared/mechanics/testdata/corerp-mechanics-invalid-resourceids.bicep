import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-invalid-resourceids'
  location: location
  properties: {
    environment: environment
  }
}

resource httpRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'invalid-rte'
  location: location
  properties: {
    application: app.location
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
