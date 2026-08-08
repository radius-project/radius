// -----------------------------------------------------------------------------
// Scenario 2 - Private Terraform module registry / repository
//
// Demonstrates authenticating to a *private* Terraform module source by attaching
// a Radius.Core/terraformSettings resource to a Radius.Core/environments. The
// terraformSettings resource renders a .terraformrc (credentials + provider
// installation) at recipe execution time and points Terraform at it via
// TF_CLI_CONFIG_FILE.
//
// The registry token is stored in a Radius.Security/secrets resource. The
// terraformSettings credentials block references that secret by ID; the
// Terraform driver resolves the secret's 'token' key and renders it into the
// generated .terraformrc.
//
// Replace the parameters below with the details of your own private registry.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-tf-demo'

@description('Kubernetes namespace the application environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-tf-demo'

@description('Kubernetes namespace the secrets environment deploys into. Must already exist and differ from kubernetesNamespace.')
param secretsNamespace string = 'private-tf-demo-secrets'

@description('Hostname of the private Terraform registry that requires a token, e.g. "app.terraform.io" or "registry.mycompany.com".')
param terraformRegistryHostname string

@description('URL of the private Terraform module source (recipeLocation). For example a module published to a private registry or an HTTP module archive.')
param recipeLocation string

@description('Token used to authenticate to the private Terraform registry.')
@secure()
param terraformRegistryToken string

@description('Name of the Redis cache the example Terraform recipe provisions.')
param redisCacheName string = 'tf-private-redis'

// Secrets environment. It carries NO recipePacks, so `rad deploy` injects the
// cluster's default recipe pack, which registers the Radius.Security/secrets
// Kubernetes recipe. That recipe materializes the backing Kubernetes Secret the
// Terraform driver later reads. It uses a separate namespace from the
// application environment because Radius rejects two environments that share a
// namespace.
resource secretsEnv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'private-tf-secrets-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: secretsNamespace
      }
    }
  }
}

// Radius.Security/secrets holding the Terraform registry token. The
// terraformSettings credentials block references this by ID; the backing
// Kubernetes Secret must expose a 'token' key, which the driver reads.
resource registryTokenSecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'private-tf-token-secret'
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

// TerraformSettings that:
//   * authenticates to the private Terraform registry (credentials block), and
//   * keeps provider installation on the default "direct" path (fetch providers
//     from the public registry). Swap "direct" for "networkMirror" if your
//     providers also live behind a private mirror.
//   * injects env vars into the Terraform process (here, raise the log level).
resource tfSettings 'Radius.Core/terraformSettings@2025-08-01-preview' = {
  name: 'private-tf-settings'
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
        kind: 'terraform'
        source: recipeLocation
      }
    }
  }
}

// Environment that references both the recipe pack and the Terraform settings.
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
    terraformSettings: tfSettings.id
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
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
