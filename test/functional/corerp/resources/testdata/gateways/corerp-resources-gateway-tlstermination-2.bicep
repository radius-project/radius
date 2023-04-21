import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-tlstermination'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    tls: { 
      minimumProtocolVersion: '1.2'
      certificateFrom: 'tlstermination-secret'
    } 
    routes: [
      {
        path: '/'
        destination: frontendRoute.id
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-front-rte'
  location: location
  properties: {
    application: app.id
    port: 443
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'tls-gtwy-front-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}
