extension radius

@description('The registry that holds the test recipes.')
param registry string

@description('The version of the test recipes.')
param version string

// Registers the recipe that names its ConfigMap after the physicalName property.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'renamableconfigmap-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/renamableConfigMaps': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_configmap_recipe:${version}'
      }
    }
  }
}
