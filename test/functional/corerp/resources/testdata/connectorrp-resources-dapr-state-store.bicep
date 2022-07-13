import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'corerp-resources-dapr-state-store'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource statestore 'Applications.Connector/daprStateStores@2022-03-15-privatepreview' = {
  name: 'statestore'
  location: 'global'

  properties: {
    environment: environment
    kind: 'generic'
    type: 'state.zookeeper'
    metadata: {
      servers: 'zookeeper.default.svc.cluster.local:2181'
    }
    version: 'v1'
  }
}
