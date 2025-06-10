import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the magpie image for the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string = 'test'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-resources'
  location: location
  properties: {
    environment: environment
  }
}

resource ctnr 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-resources'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
        }
      }
      resources: {
        requests: {
          cpu: '100m'
          memory: '128Mi'
        }
        limits: {
          cpu: '500m'
          memory: '512Mi'
        }
      }
    }
  }
}