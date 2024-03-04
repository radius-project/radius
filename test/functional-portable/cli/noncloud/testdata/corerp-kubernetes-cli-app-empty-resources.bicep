import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'kubernetes-cli-empty-resources'
  location: location
  properties: {
    environment: environment
  }
}
