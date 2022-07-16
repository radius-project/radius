import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-statestore-generic'
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
      daprstatestore: {
        source: statestore.id
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

resource statestore 'Applications.Connector/daprStateStores@2022-03-15-privatepreview' = {
  name: 'statestore-generic'
  location: location
  properties: {
    application: app.id
    environment: environment
    kind: 'generic'
    type: 'state.zookeeper'
    metadata: {
      servers: 'zookeeper.default.svc.cluster.local:2181'
    }
    version: 'v1'     
  }
}
