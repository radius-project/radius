import radius as radius

param scope string = resourceGroup().id

param registry string 

param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-recipe-env' 
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Link/redisCaches':{
        rediscache: {
          templatePath: '${registry}/test/functional/corerp/recipes/redis-recipe-value-backed:${version}' 
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-redis-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-redis-recipe-app'
      }
    ]
  }
}

resource redis 'Applications.Link/redisCaches@2022-03-15-privatepreview' = {
  name: 'rds-recipe'
  location: 'global'
  properties: {
    environment: env.id
    application: app.id
  }
}
