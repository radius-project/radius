extension radius

param registry string

param version string

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the port the container listens on.')
param port int = 8080

// Custom recipe pack that registers the user-defined type recipe. It is attached
// to the preview environment alongside the default recipe pack (which provides the
// Radius.Compute/containers recipe) via `rad env update --preview --recipe-packs`.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'usertypealpha-recipe-pack'
  location: location
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_recipe:${version}'
        parameters: {
          port: port
        }
      }
    }
  }
}
