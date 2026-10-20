extension radius

param magpieimage string
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-versioning'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'friendly-ctnr'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    containers: {
      friendlyctnr: {
        image: magpieimage
        env: {}
      }
    }
  }
}
