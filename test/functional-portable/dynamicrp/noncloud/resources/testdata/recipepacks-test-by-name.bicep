extension radius
extension testresources
extension kubernetes with {
  kubeConfig: ''
  namespace: 'recipepacks-byname-namespace'
} as kubernetes

param registry string

param version string

@description('Specifies the port the container listens on.')
param port int = 8080

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'test-recipe-pack-byname'
  location: 'global'
  properties: {
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
  name: 'recipepacks-byname-env'
  location: 'global'
  properties: {
    // Reference the recipe pack by name. The server resolves the bare name against
    // the environment's own plane and resource group. The name is a compile-time
    // constant, so an explicit dependsOn is required to deploy the pack first.
    recipePacks: [
      recipepack.name
    ]
    providers: {
     kubernetes: {
        namespace: 'recipepacks-byname-ns'
     }
    }
    recipeParameters: {
      'Test.Resources/userTypeAlpha': {
        port: 9090
      }
    }
  }
  dependsOn: [
    recipepack
  ]
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'recipepacks-byname-app'
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
