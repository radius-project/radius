import radius as radius

param magpieimage string
param environment string

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-container-versioning'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'friendly-ctnr'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {}
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    connections: {}
  }
}
