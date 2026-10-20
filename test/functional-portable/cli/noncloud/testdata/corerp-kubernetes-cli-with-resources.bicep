extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'kubernetes-cli-with-resources'
  location: location
  properties: {
    environment: environment
  }
}

resource containera 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containera-app-with-resources'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      main: {
        image: magpieimage
      }
    }
  }
}

resource containerb 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerb-app-with-resources'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      main: {
        image: magpieimage
      }
    }
  }
}
