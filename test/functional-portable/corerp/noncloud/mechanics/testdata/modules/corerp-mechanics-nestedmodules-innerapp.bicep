import radius as radius

param location string
param environment string

resource innerApp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-nestedmodules-innerapp-app'
  location: location
  properties: {
    environment: environment
  }
}
