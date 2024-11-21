extension radius

resource basic 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'basic'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default'
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
