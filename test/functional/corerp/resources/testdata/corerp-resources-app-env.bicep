import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-04-15-preview' = {
  name: 'corerp-resources-app-env-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-app-env-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-app-env-app'
  location: location
  properties: {
    environment: env.id
  }
}
