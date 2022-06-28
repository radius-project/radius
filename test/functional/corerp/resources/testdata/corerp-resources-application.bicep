import radius as radius

param location string = 'local'
param environment string = 'kind-kind'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-application-app'
  location: location
  properties: {
    environment: environment
  }
}
