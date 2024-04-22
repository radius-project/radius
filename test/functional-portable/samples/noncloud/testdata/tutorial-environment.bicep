import radius as radius

param registry string
param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'tutorial'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'tutorial'
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
