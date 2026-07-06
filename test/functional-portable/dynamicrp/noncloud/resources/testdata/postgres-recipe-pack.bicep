extension radius

param registry string

param version string

// Custom recipe pack that registers the postgres recipe. It is attached to the
// preview environment alongside the default recipe pack (which provides the
// Radius.Compute/containers recipe) via `rad env update --preview --recipe-packs`.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'dynamicrp-postgres-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/postgres': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_postgress_recipe:${version}'
      }
    }
  }
}
