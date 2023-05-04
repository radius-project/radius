import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-secretstore'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-secretstore-app'
      }
    ]
  }
}

// Reference the existing `secret-app-existing-secret` secret.
resource existingAppCert 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
  name: 'existing-appcert'
  properties:{
    application: app.id
    type: 'certificate'
    data: {
      'tls.crt': {}
      'tls.key': {}
    }
    resource: 'secret-app-existing-secret'
  }
}
