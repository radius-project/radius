
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
  name: 'corerp-resources-container-manualscale'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-manualscale-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'ctnr-manualscale'
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
      }
      extensions: [
        {
         kind: 'manualScaling'
         replicas:3 
        }   
      ]
    }
  }
