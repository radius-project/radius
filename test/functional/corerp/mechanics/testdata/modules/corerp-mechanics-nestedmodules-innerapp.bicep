import radius as radius

param location string
param environment string

resource innerApp 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-nestedmodules-innerapp-app'
  location: location
  properties: {
    environment: environment
  }
}
