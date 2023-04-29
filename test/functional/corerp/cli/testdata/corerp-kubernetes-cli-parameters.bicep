import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image of the container resource.')
param magpieimage string

resource parametersApp 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'kubernetes-cli-params'
  location: location
  properties: {
    environment: environment
  }
}

resource containerc 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerC'
  location: location
  properties: {
    application: parametersApp.id
    container: {
      image: magpieimage
    }
  }
}

resource containerd 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerD'
  location: location
  properties: {
    application: parametersApp.id
    container: {
      image: magpieimage
    }
  }
}

