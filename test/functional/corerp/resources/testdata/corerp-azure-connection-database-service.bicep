import radius as radius

param magpieimage string

param environment string

param location string = resourceGroup().location

param resourceIdentifier string = newGuid()

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
        source: databaseAccount.id
        iam: {
          kind: 'azure'
        }
      }
    }
  }
}

resource databaseAccount 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'dbacc-${resourceIdentifier}'
  location: location
  kind: 'MongoDB'
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }
}
