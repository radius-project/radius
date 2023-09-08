import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-invalid-resourceids'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
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

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'invalid-ctnr'
  location: location
  properties: {
    application: '/subscriptions/00000000-0000-0000-0000-000000000000/resourcegroups/default/providers/applications.core/environments/env'
    container: {
      image: magpieimage
    }
  }
}
