
import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-container-live-ready'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-live-ready-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'ctnr-live-ready'
  location: location
  properties: {
      application: app.id
      container: {
        image: magpieimage
        readinessProbe:{
          kind: 'httpGet'
          containerPort: 3000
          path: '/healthz'
          initialDelaySeconds:3
          failureThreshold:4
          periodSeconds:10
        }
        livenessProbe:{
          kind:'exec'
          command:'ls /tmp'
          failureThreshold:5
          initialDelaySeconds:2
          periodSeconds:10 
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
