import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-recipe-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-recipe-env'
    }
    recipes: {
      recipe1: {
          linkType: 'Applications.Link/mongoDatabases' 
          templatePath: 'testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1' 
      }
    }
    useDevRecipes: true
  }
}
