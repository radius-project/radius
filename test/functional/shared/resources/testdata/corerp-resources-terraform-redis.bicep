import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

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
            namespace: 'corerp-resources-terraform-redis-env'
            redis_cache_name: 'redis-cache-tf-recipe'
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-redis-app'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-terraform-redis-app'
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
