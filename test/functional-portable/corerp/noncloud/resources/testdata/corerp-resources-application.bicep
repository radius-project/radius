import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string = 'test'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-application-app'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-application-app'
      }
    ]
  }
}
