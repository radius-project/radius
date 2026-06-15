extension radius
extension testresources
extension kubernetes with {
  kubeConfig: ''
  namespace: 'directmodule-tf-namespace'
} as kubernetes

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

// This recipe pack points recipeLocation directly at a standard Terraform module
// (test/testrecipes/test-terraform-recipes/direct-kubernetes) that has NO `context`
// input variable and NO structured `result` output. Radius resolves the parameter
// expressions ({{context.*}}), executes the module, and maps the module's plain
// outputs onto the resource's properties via the `outputs` field.
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'directmodule-tf-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        recipeKind: 'terraform'
        recipeLocation: '${moduleServer}/direct-kubernetes.zip//modules'
        parameters: {
          name: '{{context.resource.name}}'
          namespace: '{{context.runtime.kubernetes.namespace}}'
          port: 6379
        }
        outputs: {
          host: 'host'
          port: 'port'
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
