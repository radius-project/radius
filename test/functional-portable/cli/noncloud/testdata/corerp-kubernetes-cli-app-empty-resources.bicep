extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'kubernetes-cli-empty-resources'
  location: location
  properties: {
    environment: environment
  }
}
