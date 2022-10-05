import radius as radius

param location string
param environmentId string

resource outerApp 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-nestedmodules-outerapp-app'
  location: location
  properties: {
    environment: environmentId
  }
}

module innerApp 'corerp-mechanics-nestedmodules-innerapp.bicep' = {
  name: 'corerp-mechanics-nestedmodules-innerapp'
  params: {
    location: location
    environment: environmentId
  }
}
