import radius as radius

param magpieimage string

@description('Specifies the environment for resources.')
param environment string

param location string = resourceGroup().location

param tablestorageresourceid string

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-dapr-statestore-tablestorage'
  location: location
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'ts-sts-ctnr'
  location: location
  properties: {
    application: app.id
    connections: {
      daprstatestore: {
        source: statestore.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe:{
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'ts-sts-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource statestore 'Applications.link/daprStateStores@2023-04-15-preview' = {
  name: 'ts-sts'
  location: location
  properties: {
    environment: environment
    application: app.id
    mode: 'resource'
    resource: tablestorageresourceid
  }
}
