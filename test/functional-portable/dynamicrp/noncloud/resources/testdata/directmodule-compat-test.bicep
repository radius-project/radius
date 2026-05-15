extension radius
extension testresources
extension kubernetes with {
  kubeConfig: ''
  namespace: 'directmodule-compat-ns'
} as kubernetes

param registry string

param version string

@description('Name of the Radius Application.')
param appName string

// Recipe pack using a traditional wrapped recipe (with context variable and result output).
// This test verifies backward compatibility — wrapped recipes should continue to work
// identically after the direct module support changes.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'directmodule-compat-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'bicep'
        location: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_recipe:${version}'
        parameters: {
          port: 8080
        }
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'directmodule-compat-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'directmodule-compat-ns'
      }
    }
    recipeParameters: {
      'Test.Resources/userTypeAlpha': {
        port: 9090
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

resource compatresource 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'directmodule-compat-resource'
  properties: {
    environment: env.id
    application: app.id
  }
}
