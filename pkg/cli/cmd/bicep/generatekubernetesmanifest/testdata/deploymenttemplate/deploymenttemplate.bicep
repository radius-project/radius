extension radius

param tag string = 'latest'
param kubernetesNamespace string = 'default'

resource parameters 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'parameters'
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
          templatePath: 'ghcr.io/myregistry:${tag}'
        }
      }
    }
  }
}
