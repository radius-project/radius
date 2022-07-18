import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param image string = 'radiusdev.azurecr.io/magpiego:latest'

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
  name: 'container'
  location: location
  properties: {
    application: app.id
    container: {
      image: image
      ports: {
        web: {
          containerPort: port
        }
      }
    }
    connections: {}
  }
}

