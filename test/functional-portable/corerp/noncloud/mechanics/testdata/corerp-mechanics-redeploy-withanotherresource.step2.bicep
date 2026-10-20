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

resource mechanicsb 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'mechanicsb'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      mechanicsb: {
        image: magpieimage
      }
    }
  }
}

resource mechanicsc 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'mechanicsc'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      mechanicsc: {
        image: magpieimage
      }
    }
  }
}
