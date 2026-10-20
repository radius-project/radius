extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 5000

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-bad-healthprobe'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'ctnr-bad-healthprobe'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      ctnrbadhealthprobe: {
        image: magpieimage
        readinessProbe: {
          httpGet: {
            path: '/bad'
            port: 5000
          }
          failureThreshold: 1
          periodSeconds: 1
        }
        livenessProbe: {
          httpGet: {
            path: '/bad'
            port: 5000
          }
          failureThreshold: 1
          periodSeconds: 1
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
