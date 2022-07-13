import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the image to be deployed.')
param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-application-app'
  location: location
  properties: {
    environment: environment
  }
}

resource containerA 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'a'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource containerB 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'b'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}
