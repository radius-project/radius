import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'
@description('The OCI registry for test Bicep recipes.')
param registry string
@description('The OCI tag for test Bicep recipes.')
param version string
@description('Private Git module source in generic git format.')
param privateGitModule string
@secure()
param pat string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-recipe-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-recipe-env'
    }
    recipeConfig: {
      terraform: {
        authentication: {
          git: {
            pat: {
              'github.com':{
                secret: moduleSecrets.id
              }
            }
          }
        }
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches':{
        recipe1: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/corerp-redis-recipe:${version}'
        }
        recipe2: {
          templateKind: 'terraform'
          templatePath: 'Azure/cosmosdb/azurerm' 
          templateVersion: '1.1.0'
        }
        recipe3: {
          templateKind: 'terraform'
          templatePath: privateGitModule
        }
      }
    }
  }
}

resource moduleSecrets 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'module-secrets'
  properties: {
    resource: 'test-namespace/github'
    type: 'generic'
    data: {
      username: {
        value: 'x-access-token'
      }
      pat: {
        value: pat
      }
    }
  }
}
