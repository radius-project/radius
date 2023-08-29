import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

var manifest = loadTextContent('manifest/basemanifest.yaml')

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-manifest'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-manifest'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ctnr-manifest'
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
    connections: {}
    runtimes: {
      kubernetes: {
        base: manifest
      }
    }
  }
}

