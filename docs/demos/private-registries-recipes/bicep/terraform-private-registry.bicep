// -----------------------------------------------------------------------------
// Scenario 2 - Private Terraform module registry / repository
//
// Demonstrates authenticating to a *private* Terraform module source by attaching
// a Radius.Core/terraformConfigs resource to a Radius.Core/environments. The
// terraformConfig renders a .terraformrc (credentials + provider_installation)
// at recipe execution time and points Terraform at it via TF_CLI_CONFIG_FILE.
//
// Replace the parameters below with the details of your own private registry.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-tf-demo'

@description('Kubernetes namespace the environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-tf-demo'

@description('Hostname of the private Terraform registry that requires a token, e.g. "app.terraform.io" or "registry.mycompany.com".')
param terraformRegistryHostname string

@description('URL of the private Terraform module source (recipeLocation). For example a module published to a private registry or an HTTP module archive.')
param recipeLocation string

@description('Token used to authenticate to the private Terraform registry.')
@secure()
param terraformRegistryToken string

@description('Name of the Redis cache the example Terraform recipe provisions.')
param redisCacheName string = 'tf-private-redis'

// SecretStore holding the Terraform registry token. The terraformConfig
// credentials block references this; the secret store must expose a 'token' key.
resource registryTokenSecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'private-tf-token-secret'
  location: 'global'
  properties: {
    resource: '${kubernetesNamespace}/private-tf-token-secret'
    type: 'generic'
    data: {
      token: {
        value: terraformRegistryToken
      }
    }
  }
}

// TerraformConfig that:
//   * authenticates to the private Terraform registry (credentials block), and
//   * keeps provider installation on the default "direct" path (fetch providers
//     from the public registry). Swap "direct" for "networkMirror" if your
//     providers also live behind a private mirror.
//   * injects env vars into the Terraform process (here, raise the log level).
resource tfConfig 'Radius.Core/terraformConfigs@2025-08-01-preview' = {
  name: 'private-tf-config'
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
          secret: registryTokenSecret.id
        }
      }
    }
    env: {
      TF_LOG: 'INFO'
      TF_REGISTRY_CLIENT_TIMEOUT: '15'
    }
  }
}

// RecipePack pointing at the Terraform module hosted in the private registry.
resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'private-tf-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        recipeKind: 'terraform'
        recipeLocation: recipeLocation
      }
    }
  }
}

// Environment that references both the recipe pack and the Terraform config.
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'private-tf-env'
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
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}

// Application resource that triggers the private Terraform recipe.
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
