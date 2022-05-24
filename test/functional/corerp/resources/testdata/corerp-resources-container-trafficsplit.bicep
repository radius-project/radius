import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the port of the container resource.')
param port int = 14001

@description('Specifies the environment for resources.')
param environment string

@description('Httpbin pods are used to test the application.')
param httpbinimage string = 'tommyniu.azurecr.io/httpbin:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-httproute'
  location: location
  properties: {
    environment: environment
  }
}

resource httpbinv1 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'httpbinv1'
  location: location
  properties: {
    application: app.id
    container: {
      image: httpbinimage
      ports: {
        web: {
          containerPort: port
          provides: httpbinroutev1.id
        }
      }
    }
    connections: {}
  }
}

resource httpbinv2 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'httpbinv2'
  location: location
  properties: {
    application: app.id
    container: {
      image: httpbinimage
      ports: {
        web: {
          containerPort: port
          provides: httpbinroutev2.id
        }
      }
    }
    connections: {}
  }
}

resource httpbinroutev1 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'httpbinroute-v1'
  location: location
  properties: {
    application: app.id
    port: port
  }
}

resource httpbinroutev2 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'httpbinroute-v2'
  location: location
  properties: {
    application: app.id
    port: port
  }
}


resource httpbin 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'httpbin'
  location: location
  properties: {
    application: app.id
    routes: [
      {
        destination: httpbinroutev1.id
        weight: 50
      }
      {
        destination:httpbinroutev2.id
        weight:50
      }
    ]
  }
}

