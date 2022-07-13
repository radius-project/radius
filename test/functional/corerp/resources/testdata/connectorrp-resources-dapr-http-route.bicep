import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'connectorrp-resources-dapr-http-route'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource httproute 'Applications.Connector/daprInvokeHttpRoutes@2022-03-15-privatepreview' = {
  name: 'httproute'
  location: 'global'

  properties: {
    environment: environment
    appId: 'test-app-id'
  }
}
