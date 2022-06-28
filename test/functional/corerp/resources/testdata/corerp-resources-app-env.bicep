import radius as radius

@description('Specifies the location for resources.')
param location string = 'westus2'

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-app-env-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'cluster'
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-app-env-app'
  location: location
  properties: {
    environment: env.id
  }
}
