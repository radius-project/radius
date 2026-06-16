// -----------------------------------------------------------------------------
// Scenario 3 - Combined: one environment, private Terraform AND private Bicep
//
// A single Radius.Core/environments references BOTH a Radius.Core/terraformConfigs
// (private Terraform registry credentials + env vars) and a Radius.Core/bicepConfigs
// (private OCI/Bicep registry BasicAuth). This mirrors a platform team that
// defines registry authentication once and reuses it across recipe kinds.
//
// Replace the parameters below with the details of your own private registries.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-combined-demo'

@description('Kubernetes namespace the environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-combined-demo'

// --- Private Terraform registry inputs ---
@description('Hostname of the private Terraform registry that requires a token.')
param terraformRegistryHostname string

@description('URL of the private Terraform module source (recipeLocation).')
param terraformRecipeLocation string

@description('Token used to authenticate to the private Terraform registry.')
@secure()
param terraformRegistryToken string

@description('Name of the Redis cache the example Terraform recipe provisions.')
param redisCacheName string = 'tf-combined-redis'

// --- Private Bicep (OCI) registry inputs ---
@description('Hostname of the private OCI registry that stores Bicep recipes.')
param bicepRegistryHostname string

@description('Username used for BasicAuth against the private OCI registry.')
@secure()
param bicepRegistryUsername string

@description('Password used for BasicAuth against the private OCI registry.')
@secure()
param bicepRegistryPassword string

// SecretStore for the Terraform registry token (key: token).
resource terraformTokenSecret 'Radius.Security/secretStores@2025-08-01-preview' = {
  name: 'combined-tf-token-secret'
  location: 'global'
  properties: {
    resource: '${kubernetesNamespace}/combined-tf-token-secret'
    type: 'generic'
    data: {
      token: {
        value: terraformRegistryToken
      }
    }
  }
}

// SecretStore for the Bicep OCI registry BasicAuth (keys: username, password).
resource bicepRegistrySecret 'Radius.Security/secretStores@2025-08-01-preview' = {
  name: 'combined-bicep-registry-secret'
  location: 'global'
  properties: {
    resource: '${kubernetesNamespace}/combined-bicep-registry-secret'
    type: 'generic'
    data: {
      username: {
        value: bicepRegistryUsername
      }
      password: {
        value: bicepRegistryPassword
      }
    }
  }
}

resource tfConfig 'Radius.Core/terraformConfigs@2025-08-01-preview' = {
  name: 'combined-tf-config'
  location: 'global'
  properties: {
    terraformrc: {
      providerInstallation: {
        direct: {
          include: [
            '*/*'
          ]
        }
      }
      credentials: {
        '${terraformRegistryHostname}': {
          secret: terraformTokenSecret.id
        }
      }
    }
    env: {
      TF_LOG: 'INFO'
    }
  }
}

resource bicepConfig 'Radius.Core/bicepConfigs@2025-08-01-preview' = {
  name: 'combined-bicep-config'
  location: 'global'
  properties: {
    registryAuthentications: {
      '${bicepRegistryHostname}': {
        authenticationMethod: 'BasicAuth'
        basicAuthSecretId: bicepRegistrySecret.id
      }
    }
  }
}

resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'combined-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        recipeKind: 'terraform'
        recipeLocation: terraformRecipeLocation
      }
    }
  }
}

// One environment, two config resources, both reused from a single definition.
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'combined-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: kubernetesNamespace
      }
    }
    terraformConfig: tfConfig.id
    bicepConfig: bicepConfig.id
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

resource demo 'Applications.Core/extenders@2023-10-01-preview' = {
  name: '${appName}-resource'
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
