extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-application-simple1'
  location: location
  properties: {
    environment: environment
  }
}

resource frontendContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'http-front-ctnr-simple1'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      'http-front-ctnr-simple1': {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
        readinessProbe: {
          httpGet: {
            path: '/healthz'
            port: port
          }
        }
      }
    }
    connections: {
      backend: {
        source: backendContainer.id
      }
    }
  }
}

resource backendContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'http-back-ctnr-simple1'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      'http-back-ctnr-simple1': {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
        readinessProbe: {
          httpGet: {
            path: '/healthz'
            port: port
          }
        }
      }
    }
  }
}
