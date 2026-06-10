extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'corerp-resources-app-env-env'
  location: location
  properties: {
    providers: {
      kubernetes: {
        namespace: 'corerp-resources-app-env'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-app-env-app'
  location: location
  properties: {
    environment: env.id
  }
}
