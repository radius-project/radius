import radius as radius

param scope string = resourceGroup().id

param registry string 

param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-env-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dsrp-resources-env-recipe-env' 
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches':{
        rediscache: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/redis-recipe-value-backed:${version}' 
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dsrp-resources-redis-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'dsrp-resources-redis-recipe-app'
      }
    ]
  }
}

resource redis 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'rds-recipe'
  location: 'global'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 'rediscache'
    }
  }
}
