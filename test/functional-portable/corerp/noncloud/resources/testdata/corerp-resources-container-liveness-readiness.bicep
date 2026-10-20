extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-live-ready'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'ctnr-live-ready'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      ctnrliveready: {
        image: magpieimage
        readinessProbe: {
          httpGet: {
            path: '/healthz'
            port: 3000
          }
          initialDelaySeconds: 3
          failureThreshold: 4
          periodSeconds: 10
        }
        livenessProbe: {
          exec: {
            command: ['ls', '/tmp']
          }
          initialDelaySeconds: 2
          failureThreshold: 5
          periodSeconds: 10
        }
        ports: {
          web: {
            containerPort: port
          }
        }
      }
    }
  }
}
