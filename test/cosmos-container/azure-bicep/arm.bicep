// Heavily simplified based on: https://raw.githubusercontent.com/Azure/bicep/main/docs/examples/101/cosmosdb-webapp/main.bicep

resource cosmos 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'db'
  location: resourceGroup().location
  kind: 'GlobalDocumentDB'
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
  
resource farm 'Microsoft.Web/serverFarms@2020-06-01' = {
  name: 'hosting-plan'
  location: resourceGroup().location
  sku: {
    name: 'F1'
    capacity: 1
  }
  properties: {}
}
  
resource website 'Microsoft.Web/sites@2020-06-01' = {
  name: 'my-webapp'
  location: resourceGroup().location
  properties: {
    serverFarmId: farm.id
    siteConfig: {
      appSettings: [
        {
          name: 'CosmosDb:Account'
          value: cosmos.properties.documentEndpoint
        }
        {
          name: 'CosmosDb:Key'
          value: listKeys(cosmos.id, cosmos.apiVersion).primaryMasterKey
        }
        {
          name: 'CosmosDb:DatabaseName'
          value: 'Tasks'
        }
        {
          name: 'CosmosDb:ContainerName'
          value: 'Items'
        }
      ]
    }
  }
}

resource srcCtrl 'Microsoft.Web/sites/sourcecontrols@2020-06-01' = {
  name: '${website.name}/web'
  properties: {
    repoUrl: 'https://github.com/Azure-Samples/cosmos-dotnet-core-todo-app.git'
    branch: 'main'
    isManualIntegration: true
  }
}
