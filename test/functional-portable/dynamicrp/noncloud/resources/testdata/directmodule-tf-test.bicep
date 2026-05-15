extension radius
extension testresources
extension kubernetes with {
  kubeConfig: ''
  namespace: 'directmodule-tf-ns'
} as kubernetes

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

// Recipe pack with a direct Terraform module (no wrapped result output).
// Uses outputs mapping to map module outputs to resource properties and
// parameters with {{context.*}} expressions for expression resolution testing.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'directmodule-tf-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'terraform'
        location: '${moduleServer}/kubernetes-redis.zip//modules'
        parameters: {
          redis_cache_name: '{{context.resource.name}}-cache'
          namespace: '{{context.runtime.kubernetes.namespace}}'
        }
        outputs: {
          host: 'kubernetes_deployment_name'
        }
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'directmodule-tf-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'directmodule-tf-ns'
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

resource directmoduleresource 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'directmodule-tf-resource'
  properties: {
    environment: env.id
    application: app.id
  }
}
