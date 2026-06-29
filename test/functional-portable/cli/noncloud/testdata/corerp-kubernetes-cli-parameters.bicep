extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the tag of the image to be deployed.')
param magpietag string = 'latest'

@description('Specifies the registry of the image to be deployed.')
param registry string

resource parametersApp 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'kubernetes-cli-params'
  location: location
  properties: {
    environment: environment
  }
}

resource containerc 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerc'
  location: location
  properties: {
    application: parametersApp.id
    environment: environment
    containers: {
      main: {
        image: '${registry}/magpiego:${magpietag}'
      }
    }
  }
}

resource containerd 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerd'
  location: location
  properties: {
    application: parametersApp.id
    environment: environment
    containers: {
      main: {
        image: '${registry}/magpiego:${magpietag}'
      }
    }
  }
}

