extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'corerp-resources-environment-2025-env'
  location: location
  properties: {
    providers: {
      kubernetes: {
        namespace: 'corerp-resources-environment-2025-env'
      }
    }
  }
}
