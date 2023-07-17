import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for the resource.')
param environment string = 'test'

module outerApp 'modules/corerp-mechanics-nestedmodules-outerapp.bicep' = {
  name: 'corerp-mechanics-nestedmodules-outerapp'
  params: {
    location: location
    environment: environment
  }
}
