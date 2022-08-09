import radius@0.13 as radius

param location string = resourceGroup().location
param accountName string = 'todoapp-cosmos-${uniqueString(resourceGroup().id)}'

resource app 'Applications.Core/applications' = {
  name: 'todoapp'
  properties: {
    environment: radius.environment
  }
}

resource todoFrontend 'Applications.Core/containers' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: {
      image: 'radius.azurecr.io/webapptutorial-todoapp'
    }
    connections: {
      mongodb: {
        source: db.id
      }
    }
  } 
}

resource db 'Applications.Connector/mongoDatabases' = {
  name: 'db'
  properties: {
    environment: radius.environment
    resource: cosmosAccount::cosmosDb.id
  }
}

resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: toLower(accountName)
  location: location
  kind: 'MongoDB'
  properties: {
    databaseAccountOfferType: 'Standard'
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
      }
    ]
  }
  

  resource cosmosDb 'mongodbDatabases' = {
    name: 'db'
    properties: {
      resource: {
        id: 'db'
      }
      options: {
        throughput: 400
      }
    }
  }
}
