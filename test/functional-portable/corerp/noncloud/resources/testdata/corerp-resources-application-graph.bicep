provider radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-application-simple1'
  location: location
  properties: {
    environment: environment
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-front-ctnr-simple1'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
    connections: {
      backend: {
        source: 'http://http-back-ctnr-simple1:3000'
      }
    }
  }
}


resource backendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-back-ctnr-simple1'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
     
      ports: {
        web: {
          containerPort: port
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
  }
}
