import radius as radius

param magpieimage string

param environment string

param location string = resourceGroup().location

param documentdbresourceid string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-azure-connection-database-service'
  location: location
  properties: {
    environment: environment
  }
}

resource store 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'db-service'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      databaseresource: {
        source: documentdbresourceid
      }
    }
  }
}
