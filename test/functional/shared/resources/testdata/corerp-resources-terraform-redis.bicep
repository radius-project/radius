import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string = 'http://tf-module-server.radius-test-tf-module-server.svc.cluster.local'

@description('Name of the Redis Cache resource.')
param redisCacheName string = 'tf-redis-cache'

@description('Name of the Radius Application.')
param appName string = 'corerp-resources-terraform-redis-app'

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
          templatePath: 'http://localhost:8123/kubernetes-redis.zip'
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
    recipe: {
      name: 'default'
      parameters: {
        redis_cache_name: redisCacheName
      }
    }
  }
}
