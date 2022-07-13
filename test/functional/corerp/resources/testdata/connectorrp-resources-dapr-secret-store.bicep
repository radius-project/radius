import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'connectorrp-resources-dapr-secret-store'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource secretstore 'Applications.Connector/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'secretstore'
  location: 'global'

  properties: {
    environment: environment
    kind: 'generic'
    type: 'secretstores.kubernetes'
    metadata: {
      name: 'test'
    }
    version: 'v1'
  }
}
