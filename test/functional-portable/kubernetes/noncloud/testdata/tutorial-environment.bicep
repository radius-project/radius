extension radius

param name string
param namespace string
param registry string
param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: name
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: namespace
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/redis-recipe-value-backed:${version}'
        }
      }
    }
  }
}
