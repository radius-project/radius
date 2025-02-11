extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'radius-test-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'radius-test-env-ns-1'
    }
  }
}
