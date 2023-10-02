import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-recipe-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-recipe-env'
    }
    recipes: {
      'Applications.Datastores/mongoDatabases':{
        recipe1: {
          templateKind: 'bicep'
          templatePath: 'testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1' 
        }
        recipe2: {
          templateKind: 'terraform'
          templatePath: 'Azure/cosmosdb/azurerm' 
          templateVersion: '1.1.0'
        }
      }
    }
  }
}
