extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string = 'test'

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-application-2025-app'
  location: location
  properties: {
    environment: environment
  }
}
