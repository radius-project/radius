import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('The namespace to deploy the Redis cache to.')
param namespace string

@description('Name of the Redis Cache resource.')
param redisCacheName string

@description('Name of the Radius Application.')
param appName string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-redis-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-redis-env'
    }
    recipes: {
      'Applications.Link/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/kubernetes-redis.zip'
          parameters: {
            namespace: namespace // This will be replaced by context parameter after it is implemented
            redis_cache_name: redisCacheName
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
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

resource webapp 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-redis'
  properties: {
    application: app.id
    environment: env.id
  }
}
