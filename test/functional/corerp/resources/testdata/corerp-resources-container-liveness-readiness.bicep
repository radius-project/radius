
import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
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
          periodSeconds:20
        }
        /*
        livenessProbe:{
          kind:'exec'
          command:'ls /tmp'
          failureThreshold:5
        }
        */
        ports: {
          web: {
            containerPort: port
          }
        }
      }
      connections: {}
    }
  }
/*
  "livenessProbe": {
    "kind": "tcp",
    "tcp": {
      "healthProbeBase": {
        "failureThreshold": 5,
        "initialDelaySeconds": 5,
        "periodSeconds": 5
      },
      "containerPort": 8080
    }
  }
*/

