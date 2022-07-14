import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param environment string
param location string = resourceGroup().location

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
          roles: [
            'Cosmos DB Account Reader Role'
            '230815da-be43-4aae-9cb4-875f7bd000aa'
          ]
        }
      }
    }
  }
}

resource databaseAccount 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'dbacc-${guid(resourceGroup().name)}'
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
