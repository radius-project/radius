extension radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Redis Cache resource.')
param redisCacheName string

@description('Name of the Radius Application.')
param appName string

@description('Name of the Radius Environment.')
param envName string = 'corerp-resources-terraform-redis-env'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: envName
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: envName
    }
    recipeConfig: {
      env: {
        MY_ENV_VAR_1: 'env-var-value-1'
        MY_ENV_VAR_2: 'env-var-value-2'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/kubernetes-redis.zip//modules'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: appName
      }
    ]
  }
}

resource webapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-redis'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'default'
      parameters: {
        redis_cache_name: redisCacheName
      }
    }
  }
}
