extension radius

param name string
param namespace string
param registry string
param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: '${name}-env'
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

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: '${name}-app'
  properties: {
    environment: env.id
  }
}

resource recipe 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: '${name}-recipe'
  properties: {
    application: app.id
    environment: env.id
  }
}
