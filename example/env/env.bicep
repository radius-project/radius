// Import the set of Radius resources (Applications.*) into Bicep
extension radius

param kubernetesNamespace string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: kubernetesNamespace
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        testrecipe: {
          templateKind: 'bicep'
          templatePath: 'ghcr.io/radius-project/recipes/local-dev/rediscaches:0.36'
        }
      }
    }
  }
}
