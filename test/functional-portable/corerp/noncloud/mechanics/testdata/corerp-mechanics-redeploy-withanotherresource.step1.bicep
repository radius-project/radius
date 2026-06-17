extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-mechanics-redeploy-with-another-resource'
  location: location
  properties: {
    environment: environment
  }
}

resource mechanicsa 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'mechanicsa'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      mechanicsa: {
        image: magpieimage
      }
    }
  }
}
