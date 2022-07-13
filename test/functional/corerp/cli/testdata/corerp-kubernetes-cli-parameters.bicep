import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = '/planes/radius/local/resourcegroups/ucpenv-rg/providers/applications.core/environments/ucpenv'

@description('Specifies the tag of the image to be deployed.')
param magpietag string = 'latest'

@description('Specifies the registry of the image to be deployed.')
param registry string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'kubernetes-cli'
  location: location
  properties: {
    environment: environment
  }
}

resource a 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'a'
  location: location
  properties: {
    application: app.id
    container: {
      image: '${registry}/magpiego:${magpietag}'
    }
  }
}

resource b 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'b'
  location: location
  properties: {
    application: app.id
    container: {
      image: '${registry}/magpiego:${magpietag}'
    }
  }
}

