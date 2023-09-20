import radius as radius

param location string
param environment string

resource outerApp 'Applications.Core/applications@2023-10-01-preview' = {
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
