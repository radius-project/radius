extension radius
extension testresources

param registry string

param version string

@description('Specifies the port the container listens on.')
param port int = 8080

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'test-recipe-pack-no-provider'
  location: 'global'
  properties: {
    description: 'Test recipe pack with userTypeAlpha recipe'
    recipes: {
      'Test.Resources/userTypeAlpha': {
        recipeKind: 'bicep'
        recipeLocation: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_recipe:${version}'
        parameters: {
          port: port
        }
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'recipepacks-test-env-no-provider'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    // No providers block - this should cause deployment failure
    // since the recipe also does not have a namespace  created or configured
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'recipepacks-test-app-no-provider'
  location: 'global'
  properties: {
    environment: env.id
  }
}

resource rrtresource 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'rrtresource'
  properties: {
    environment: env.id
    application: app.id
  }
}
