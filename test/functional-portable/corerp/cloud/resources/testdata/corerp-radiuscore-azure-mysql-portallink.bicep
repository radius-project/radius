extension radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

@description('Per-run seed used to ensure the Azure MySQL server name does not collide across concurrent CI runs that share a test subscription.')
param uniqueSeed string = ''

@description('Azure subscription hosting the Azure resources provisioned by the mysql recipe.')
param azureSubscriptionId string

@description('Azure resource group hosting the Azure resources provisioned by the mysql recipe.')
param azureResourceGroupName string

@secure()
@description('Administrator password set on the Radius.Data/mySqlDatabases resource (required by the resource schema). The Azure MySQL recipe generates its own Azure-compliant admin credentials and does not consume this value.')
param password string

// Recipe pack referencing the mysql-azure.zip recipe shipped from the repo
// test terraform module server.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-mysql-portallink-pack'
  location: 'global'
  properties: {
    recipes: {
      'Radius.Data/mySqlDatabases': {
        kind: 'terraform'
        source: '${moduleServer}/mysql-azure.zip'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'azure-mysql-portallink-env-${uniqueSeed}'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'azure-mysql-portallink-ns'
      }
      azure: {
        subscriptionId: azureSubscriptionId
        resourceGroupName: azureResourceGroupName
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

// The mysql recipe provisions a real Azure MySQL Flexible Server (+ firewall
// rule + database). The Terraform driver auto-discovers azurerm_* resources
// from the tf state and records their ARM IDs into status.outputResources,
// which the application graph then decorates with an Azure portal deep link.
//
// username/password are required by the resource schema. The Azure MySQL recipe
// generates its own Azure-compliant admin credentials (Azure rejects common
// names such as "admin"), so these values are not consumed by the recipe.
resource mysql 'Radius.Data/mySqlDatabases@2025-08-01-preview' = {
  name: 'azure-mysql-portallink-db-${uniqueSeed}'
  properties: {
    environment: env.id
    application: app.id
    username: 'mysqladmin'
    password: password
  }
}
