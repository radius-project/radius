extension radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

@description('The ID of the preview environment that provisions the Radius.Security/secrets resource.')
param secretsEnvironment string

@description('Name of the Redis Cache resource provisioned by the Terraform recipe.')
param redisCacheName string = 'tfsec-redis-cache'

// Registry-credential secret. It is provisioned into the preview environment, whose default
// recipe pack registers the Radius.Security/secrets Kubernetes recipe. The recipe materializes a
// same-named Kubernetes Secret (holding the 'token' key) in the preview environment namespace.
// This backing Secret is what the recipe secret loader dereferences when resolving the
// terraformSettings credentials below.
resource registryToken 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'tfsec-registry-token'
  location: 'global'
  properties: {
    environment: secretsEnvironment
    kind: 'generic'
    data: {
      token: {
        value: 'test-token-value'
      }
    }
  }
}

// terraformSettings referencing the Radius.Security/secrets resource for private Terraform registry
// credentials. This is the path that previously rejected Radius.Security/secrets (only
// Applications.Core/secretStores was accepted). The Terraform driver resolves the referenced
// secret's 'token' key and renders it into the generated .terraformrc at recipe-execution time.
resource tfConfig 'Radius.Core/terraformSettings@2025-08-01-preview' = {
  name: 'tfsec-terraform-config'
  location: 'global'
  properties: {
    terraformrc: {
      credentials: {
        'app.terraform.io': {
          secret: registryToken.id
        }
      }
    }
  }
}

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'tfsec-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        recipeKind: 'terraform'
        recipeLocation: '${moduleServer}/kubernetes-redis.zip//modules'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'tfsec-redis-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'tfsec-redis-ns'
      }
    }
    terraformSettings: tfConfig.id
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

// Deploying this extender runs the Terraform recipe, which forces the secret loader to resolve the
// terraformSettings credential. If Radius.Security/secrets were not supported, recipe setup would fail
// when loading the secret; a successful deployment proves the new secret type is accepted end-to-end.
resource webapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'tfsec-redis-extender'
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
