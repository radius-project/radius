import radius as radius

param magpieimage string

param environment string

param cosmosmongodbresourceid string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-azure-connection-database-service'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource store 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'db-service'
  location: 'global'
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      databaseresource: {
        source: cosmosmongodbresourceid
      }
    }
  }
}
