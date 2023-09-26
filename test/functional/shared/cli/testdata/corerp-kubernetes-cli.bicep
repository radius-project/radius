import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'kubernetes-cli'
  location: location
  properties: {
    environment: environment
  }
}

resource containera 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerA'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource containerb 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerB'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}
