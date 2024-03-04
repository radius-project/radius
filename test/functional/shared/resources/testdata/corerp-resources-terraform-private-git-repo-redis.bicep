import radius as radius

@description('Name of the Redis Cache resource.')
param redisCacheName string

@description('Name of the Radius Application.')
param appName string

@secure()
param pat string=''

@description('Private Git module source in generic git format.')
param privateGitModule string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-private-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-private-env'
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: privateGitModule
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
  name: 'corerp-resources-terraform-private-redis'
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

resource moduleSecrets 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'module-secrets'
  properties: {
    resource: 'test-namespace/github'
    type: 'generic'
    data: {
      pat: {
        value: pat 
      }
    }
  }
}
