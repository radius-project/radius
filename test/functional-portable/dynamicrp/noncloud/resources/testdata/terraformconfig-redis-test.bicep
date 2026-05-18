extension radius
extension kubernetes with {
  kubeConfig: ''
  namespace: 'tfconfig-redis-ns'
} as kubernetes

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Redis Cache resource.')
param redisCacheName string = 'tf-redis-cache'

@description('Name of the Radius Application.')
param appName string

resource tfConfig 'Radius.Core/terraformConfigs@2025-08-01-preview' = {
  name: 'test-terraform-config'
  location: 'global'
  properties: {
    env: {
      MY_ENV_VAR_1: 'env-var-value-1'
      MY_ENV_VAR_2: 'env-var-value-2'
    }
  }
}

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'tfconfig-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        kind: 'terraform'
        location: '${moduleServer}/kubernetes-redis.zip//modules'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'tfconfig-redis-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'tfconfig-redis-ns'
      }
    }
    terraformConfig: tfConfig.id
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

resource webapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'tfconfig-redis-extender'
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
