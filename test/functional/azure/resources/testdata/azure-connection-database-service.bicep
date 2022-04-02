param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' 

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-connection-database-service'

  resource store 'Container' = {
    name: 'db-service'
    properties: {
      container: {
        image: magpieimage
      }
      connections: {
        databaseresource: {
          kind:'azure'
          source: databaseAccount.id
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
  location: resourceGroup().location
  kind: 'MongoDB'
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
      {
        locationName: resourceGroup().location
        failoverPriority: 0
        isZoneRedundant: false
      }
    ]
    databaseAccountOfferType: 'Standard'
  }
}

