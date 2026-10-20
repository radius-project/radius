extension radius

param location string
param environment string

resource innerApp 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-mechanics-nestedmodules-innerapp-app'
  location: location
  properties: {
    environment: environment
  }
}
