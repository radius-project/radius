// -----------------------------------------------------------------------------
// Scenario 3 - Combined: one environment, private Terraform AND private Bicep
//
// A single Radius.Core/environments references BOTH a Radius.Core/terraformSettings
// (private Terraform registry credentials + env vars) and a Radius.Core/bicepSettings
// (private OCI/Bicep registry BasicAuth). This mirrors a platform team that
// defines registry authentication once and reuses it across recipe kinds.
//
// Both registry credentials are stored as Radius.Security/secrets resources,
// provisioned into a dedicated secrets environment so their backing Kubernetes
// Secrets exist before the recipe drivers resolve them.
//
// Replace the parameters below with the details of your own private registries.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-combined-demo'

@description('Kubernetes namespace the application environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-combined-demo'

@description('Kubernetes namespace the secrets environment deploys into. Must already exist and differ from kubernetesNamespace.')
param secretsNamespace string = 'private-combined-demo-secrets'

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

// Secrets environment. It carries NO recipePacks, so `rad deploy` injects the
// cluster's default recipe pack, which registers the Radius.Security/secrets
// Kubernetes recipe. Both credential secrets below are provisioned here so their
// backing Kubernetes Secrets exist before the recipe drivers resolve them. It
// uses a separate namespace from the application environment because Radius
// rejects two environments that share a namespace.
resource secretsEnv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'combined-secrets-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: secretsNamespace
      }
    }
  }
}

// Radius.Security/secrets for the Terraform registry token (key: token).
resource terraformTokenSecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'combined-tf-token-secret'
  location: 'global'
  properties: {
    environment: secretsEnv.id
    kind: 'generic'
    data: {
      token: {
        value: terraformRegistryToken
      }
    }
  }
}

// Radius.Security/secrets for the Bicep OCI registry BasicAuth (keys: username, password).
resource bicepRegistrySecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'combined-bicep-registry-secret'
  location: 'global'
  properties: {
    environment: secretsEnv.id
    kind: 'generic'
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

resource tfSettings 'Radius.Core/terraformSettings@2025-08-01-preview' = {
  name: 'combined-tf-settings'
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

resource bicepSettings 'Radius.Core/bicepSettings@2025-08-01-preview' = {
  name: 'combined-bicep-settings'
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
        kind: 'terraform'
        source: terraformRecipeLocation
      }
    }
  }
}

// One environment, two settings resources, both reused from a single definition.
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
    terraformSettings: tfSettings.id
    bicepSettings: bicepSettings.id
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
