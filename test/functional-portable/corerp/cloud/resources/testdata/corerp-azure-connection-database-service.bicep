extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the resource ID of the Azure Cosmos DB for MongoDB account to connect to.')
param cosmosmongodbresourceid string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-azure-connection-database-service'
  location: location
  properties: {
    environment: environment
  }
}

resource store 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'db-service'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      'db-service': {
        image: magpieimage
      }
    }
    connections: {
      databaseresource: {
        source: cosmosmongodbresourceid
      }
    }
  }
}
