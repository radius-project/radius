extension radius

param location string
param environment string

resource outerApp 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-mechanics-nestedmodules-outerapp-app'
  location: location
  properties: {
    environment: environment
  }
}

module innerApp 'corerp-mechanics-nestedmodules-innerapp.bicep' = {
  name: 'corerp-mechanics-nestedmodules-innerapp'
  params: {
    location: location
    environment: environment
  }
}
