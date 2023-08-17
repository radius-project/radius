import radius as radius

param scope string = resourceGroup().id

param registry string

param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'dsrp-resources-env-default-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dsrp-resources-env-default-recipe-env'
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/redis-recipe-value-backed:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dsrp-resources-redis-default-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'dsrp-resources-redis-default-recipe-app'
      }
    ]
  }
}

resource redis 'Applications.Datastores/redisCaches@2022-03-15-privatepreview' = {
  name: 'rds-default-recipe'
  location: 'global'
  properties: {
    environment: env.id
    application: app.id
  }
}
