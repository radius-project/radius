import radius as radius {

}

// #1 @setDefault(radius, 'location', 'global')

// #2 @setDefault(az, 'location')
// param azLocation string = 'global'

param environment string
param location string = resourceGroup().location
param accountName string = 'todoapp-cosmos-${uniqueString(resourceGroup().id)}'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'todoapp'
  properties: {
    environment: environment
  }
}

resource todoFrontend 'Applications.Core/containers@2022-03-15-privatepreview' = {
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

resource db 'Applications.Connector/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'db'
  properties: {
    environment: environment
    resource: cosmosAccount::cosmosDb.id
  }
}

resource cosmosAccount 'Microsoft.DocumentDB/databaseAccounts@2021-04-15' = {
  name: toLower(accountName)
  //location: location
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
