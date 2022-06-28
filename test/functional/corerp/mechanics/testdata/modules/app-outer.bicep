import radius as radius

param location string = 'westus2'
param environment string = ''

resource outerApp 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-nestedmodules-outerapp-app'
  location: location
  properties: {
    environment: environment
  }
}

module innerApp 'app-inner.bicep' = {
  name: 'corerp-mechanics-nestedmodules-innerapp'
  params: {
    location: location
  }
}
