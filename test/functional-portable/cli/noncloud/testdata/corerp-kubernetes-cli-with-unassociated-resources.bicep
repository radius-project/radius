extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'kubernetes-cli-with-unassociated-resources'
  location: location
  properties: {
    environment: environment
  }
}

resource containerx 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerX'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource containery 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerY'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}
