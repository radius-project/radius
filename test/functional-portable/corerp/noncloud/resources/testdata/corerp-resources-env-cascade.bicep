extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-env-cascade'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'env-cascade-ctnr'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      envcascadectnr: {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
      }
    }
    connections: {}
  }
}
