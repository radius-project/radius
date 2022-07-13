import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-secretstore-generic'
  location: location
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'myapp'
  location: location
  properties: {
    application: app.id
    connections: {
      daprsecretstore: {
        source: secretstore.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
  }
}

resource secretstore 'Applications.Connector/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'secretstore-generic'
  location: location
  properties: {
    environment: environment
    kind: 'generic'
    type: 'secretstores.azure.keyvault'
    metadata: {
      vaultName: 'test'
    }
    version: 'v1'
  }
}
