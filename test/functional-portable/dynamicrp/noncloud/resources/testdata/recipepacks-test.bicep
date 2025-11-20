extension radius
extension testresources
extension kubernetes with {
  kubeConfig: ''
  namespace: 'recipepacks-namespace'
} as kubernetes

param registry string

param version string

@description('Specifies the port the container listens on.')
param port int = 8080

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'test-recipe-pack'
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
      'Test.Resources/postgres': {
        recipeKind: 'bicep'
        recipeLocation: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_postgress_recipe:${version}'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'recipepacks-test-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
     kubernetes: {
        namespace: 'recipepacks-ns'
     }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'recipepacks-test-app'
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
