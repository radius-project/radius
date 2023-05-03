import radius as radius

param scope string = resourceGroup().id

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-value-backed-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-value-backed-recipe-env'
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Link/redisCaches':{
        rediscache: {
          templatePath: 'radiusdev.azurecr.io/recipes/functionaltest/valuebacked/rediscaches/azure:1.0' 
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-redis-value-backed-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-redis-value-backed-recipe-app'
      }
    ]
  }
}

resource redis 'Applications.Link/redisCaches@2022-03-15-privatepreview' = {
  name: 'rds-value-backed-recipe'
  location: 'global'
  properties: {
    environment: env.id
    application: app.id
    mode: 'recipe'
    recipe: {
      name: 'rediscache'
    }
  }
}
