extension radius
extension testresources

@description('Registry hosting the published direct-module test recipe.')
param registry string

@description('Version (tag) of the published direct-module test recipe.')
param version string

@description('Name of the Radius Application.')
param appName string

// This recipe pack points source directly at a standard (non-wrapped) Bicep
// module (test/testrecipes/test-bicep-recipes/directmodule-kubernetes.bicep) that has
// NO `context` parameter and NO structured `result` output. Radius resolves the
// parameter expressions ({{context.*}}), deploys the module via the ARM deployment
// engine, and maps the module's plain outputs onto the resource's properties via the
// `outputs` field.
//
// Two recipe packs map the same module with different `lock.kind` values. The module
// creates a conditional ConfigMap only when `lock.kind` is not 'None', which mirrors the
// lock condition in Azure Verified Modules.
var directModuleSource = '${registry}/test/testrecipes/test-bicep-recipes/directmodule-kubernetes:${version}'

var directModuleParameters = {
  name: '{{context.resource.name}}'
  namespace: '{{context.runtime.kubernetes.namespace}}'
  containerPort: 6379
}

var directModuleOutputs = {
  host: 'host'
  port: 'port'
}

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'directmodule-bicep-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'bicep'
        source: directModuleSource
        parameters: union(directModuleParameters, {
          lock: {
            kind: 'None'
          }
        })
        outputs: directModuleOutputs
      }
    }
  }
}

resource lockedRecipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'directmodule-bicep-locked-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'bicep'
        source: directModuleSource
        parameters: union(directModuleParameters, {
          lock: {
            kind: 'CanNotDelete'
          }
        })
        outputs: directModuleOutputs
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'directmodule-bicep-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'directmodule-bicep-ns'
      }
    }
  }
}

// Radius rejects two environments that claim the same Kubernetes namespace, so the locked
// environment uses its own.
resource lockedEnv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'directmodule-bicep-locked-env'
  location: 'global'
  properties: {
    recipePacks: [
      lockedRecipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'directmodule-bicep-locked-ns'
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
  name: 'directmodule-bicep-resource'
  properties: {
    environment: env.id
    application: app.id
  }
}

resource lockedResource 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'directmodule-bicep-locked-resource'
  properties: {
    environment: lockedEnv.id
    application: app.id
  }
}
