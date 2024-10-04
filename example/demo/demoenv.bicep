// Import the set of Radius resources (Applications.*) into Bicep
extension radius

param kubernetesNamespace string

resource demoenv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'demoenv'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: kubernetesNamespace
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templateKind: 'bicep'
          templatePath: 'ghcr.io/radius-project/recipes/local-dev/rediscaches:latest'
        }
      }
    }
  }
}
