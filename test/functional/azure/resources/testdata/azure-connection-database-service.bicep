resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-connection-database-service'

  resource store 'Container' = {
    name: 'db-service'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
      connections: {
        translationresource: {
          kind:'azure'
          source: databaseAccount.id
          roles: [
            'Database User'
            '25fbc0a9-bd7c-42a3-aa1a-3b75d497ee68'
          ]
        }
      }
    }
  }
}

resource databaseAccount 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'TextTranslationAccount-${guid(resourceGroup().name)}'
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

