extension radius
extension kubernetes with {
  kubeConfig: ''
  namespace: 'tfbicep-combined-ns'
} as kubernetes

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Redis Cache resource.')
param redisCacheName string = 'tf-bicep-redis-cache'

@description('Name of the Radius Application.')
param appName string

// TerraformConfig with provider_installation (network mirror) and env vars.
// The mirror URL is intentionally not reachable: this test verifies that the
// resources are accepted, the environment resolves both refs, and that the
// recipe (which uses no providers from the mirror) still succeeds. This proves
// the wiring without requiring a real network mirror in the test cluster.
resource tfConfig 'Radius.Core/terraformSettings@2025-08-01-preview' = {
  name: 'tfbicep-combined-tfconfig'
  location: 'global'
  properties: {
    terraformrc: {
      providerInstallation: {
        // Direct-only block: instructs Terraform to fetch every provider
        // directly from the registry (the default behavior). This is a valid
        // .terraformrc that the driver renders and points TF_CLI_CONFIG_FILE at,
        // exercising the new code path without needing a real mirror.
        direct: {
          include: ['*/*']
        }
      }
    }
    env: {
      MY_ENV_VAR_COMBINED: 'env-var-value-combined'
    }
  }
}

// SecretStore providing username/password for the BasicAuth registry config.
// Placeholder values; the test exercises CRUD wiring, not a real registry pull.
resource registrySecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'tfbicep-combined-registry-secret'
  location: 'global'
  properties: {
    resource: 'tfbicep-combined-ns/tfbicep-combined-registry-secret'
    type: 'generic'
    data: {
      username: { value: 'test-user' }
      password: { value: 'test-pass' }
    }
  }
}

resource bicepSettings 'Radius.Core/bicepSettings@2025-08-01-preview' = {
  name: 'tfbicep-combined-bicepconfig'
  location: 'global'
  properties: {
    registryAuthentications: {
      'corp.acr.example.io': {
        authenticationMethod: 'BasicAuth'
        basicAuthSecretId: registrySecret.id
      }
    }
  }
}

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'tfbicep-combined-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        kind: 'terraform'
        source: '${moduleServer}/kubernetes-redis.zip//modules'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'tfbicep-combined-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'tfbicep-combined-ns'
      }
    }
    terraformSettings: tfConfig.id
    bicepSettings: bicepSettings.id
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
  name: 'tfbicep-combined-extender'
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
