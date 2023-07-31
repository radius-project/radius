import radius as radius

param magpieimage string

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dssm-old'
  location: location
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-ctnr-old'
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
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'gnrc-ss-ctnr-old'
        appPort: 3000
      }
    ]
  }
}

resource secretstore 'Applications.Link/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-manual-old'
  location: location
  properties: {
    environment: environment
    application: app.id
    resourceProvisioning: 'manual'
    type: 'secretstores.kubernetes'
    metadata: {
      vaultName: 'test'
    }
    version: 'v1'
  }
}
