import radius as radius

@description('Specifies the location for resources.')
param location string = 'westus2'

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-nestedmodules-env'
  location: location
  properties: {
    compute:{
      kind: 'kubernetes'
      resourceId: ''
    }
  }
}

module outerApp 'modules/app-outer.bicep' = {
  name: 'corerp-mechanics-nestedmodules-outerapp'
  params: {
    location: location
    environment: 'test'
  }
}
