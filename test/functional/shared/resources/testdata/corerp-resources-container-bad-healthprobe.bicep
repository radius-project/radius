
import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 5000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-bad-healthprobe'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-bad-healthprobe-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-bad-healthprobe'
  location: location
  properties: {
      application: app.id
      container: {
        image: magpieimage
        readinessProbe:{
          kind: 'httpGet'
          containerPort: 5000
          path: '/bad'
          failureThreshold:1
          periodSeconds:1
        }
        livenessProbe:{
          kind: 'httpGet'
          containerPort: 5000
          path: '/bad'
          failureThreshold:1
          periodSeconds:1
        }
        
        ports: {
          web: {
            containerPort: port
          }
        }
      }
      connections: {}
    }
  }
