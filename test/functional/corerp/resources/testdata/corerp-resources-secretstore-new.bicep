import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies tls cert secret values.')
@secure()
param tlscrt string
@secure()
param tlskey string

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

// Create new certificate type appcert secret.
resource appCert 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
  name: 'appcert'
  properties:{
    application: app.id
    type: 'certificate'
    data: {
      'tls.key': {
        value: tlskey
      }
      'tls.crt': {
        value: tlscrt
      }
    }
  }
}

// Create new generic type appSecret.
resource appSecret 'Applications.Core/secretStores@2022-03-15-privatepreview' = {
  name: 'appsecret'
  properties:{
    application: app.id
    data: {
      servicePrincipalPassword: {
        value: '10000000-1000-1000-0000-000000000000'
      }
      appId: {
        value: '00000000-0000-0000-0000-000000000001'
      }
      tenantId: {
        encoding: 'raw'
        value: '00000000-0000-0000-0000-000000000002'
      }
    }
  }
}
