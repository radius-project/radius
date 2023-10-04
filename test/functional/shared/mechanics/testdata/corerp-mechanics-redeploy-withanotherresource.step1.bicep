import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-redeploy-with-another-resource'
  location: location
  properties: {
    environment: environment
  }
}

resource mechanicsa 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mechanicsa'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}
