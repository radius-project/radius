import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'kubernetes-cli-with-resources'
  location: location
  properties: {
    environment: environment
  }
}

resource containera 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containera-app-with-resources'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource containerb 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerb-app-with-resources'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}
