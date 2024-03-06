import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the tag of the image to be deployed.')
param magpietag string = 'latest'

@description('Specifies the registry of the image to be deployed.')
param registry string

resource parametersApp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'kubernetes-cli-params'
  location: location
  properties: {
    environment: environment
  }
}

resource containerc 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerC'
  location: location
  properties: {
    application: parametersApp.id
    container: {
      image: '${registry}/magpiego:${magpietag}'
    }
  }
}

resource containerd 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerD'
  location: location
  properties: {
    application: parametersApp.id
    container: {
      image: '${registry}/magpiego:${magpietag}'
    }
  }
}

