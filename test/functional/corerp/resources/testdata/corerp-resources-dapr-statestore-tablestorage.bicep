import radius as radius

param magpieimage string

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-statestore-tablestorage'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ts-sts-ctnr'
  location: 'global'
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

resource statestore 'Applications.Connector/daprStateStores@2022-03-15-privatepreview' = {
  name: 'ts-sts'
  location: 'global'
  properties: {
    environment: environment
    application: app.id
    kind: 'state.azure.tablestorage'
    resource: '/subscriptions/85716382-7362-45c3-ae03-2126e459a123/resourceGroups/RadiusFunctionalTest/providers/Microsoft.Storage/storageAccounts/tsaccountradiustest/tableServices/default/tables/radiustest'
  }
}
