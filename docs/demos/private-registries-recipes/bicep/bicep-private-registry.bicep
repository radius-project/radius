// -----------------------------------------------------------------------------
// Scenario 1 - Private Bicep recipe registry (OCI, e.g. Azure Container Registry)
//
// Demonstrates pulling a Bicep Recipe from a *private* OCI registry by attaching
// a Radius.Core/bicepSettings resource (BasicAuth) to a Radius.Core/environments.
//
// Registry credentials are stored in a Radius.Security/secrets resource. The
// bicepSettings resource references that secret by ID; at recipe-execution time
// the Bicep driver resolves the referenced secret and authenticates to the
// registry using the method configured on bicepSettings (BasicAuth here).
//
// Replace the parameters below with the details of your own private registry.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-bicep-demo'

@description('Kubernetes namespace the application environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-bicep-demo'

@description('Kubernetes namespace the secrets environment deploys into. Must already exist and differ from kubernetesNamespace.')
param secretsNamespace string = 'private-bicep-demo-secrets'

@description('Hostname of the private OCI registry that stores the Bicep recipe, e.g. "myregistry.azurecr.io".')
param registryHostname string

@description('Full OCI path to the published Bicep recipe, e.g. "myregistry.azurecr.io/recipes/myredis:latest".')
param recipeLocation string

@description('Username used for BasicAuth against the private registry (for ACR this can be an ACR token name or a service principal app ID).')
@secure()
param registryUsername string

@description('Password used for BasicAuth against the private registry (for ACR this can be an ACR token password or a service principal secret).')
@secure()
param registryPassword string

// Secrets environment. It carries NO recipePacks, so `rad deploy` injects the
// cluster's default recipe pack, which registers the Radius.Security/secrets
// Kubernetes recipe. That recipe materializes the backing Kubernetes Secret the
// Bicep driver later reads. It uses a separate namespace from the application
// environment because Radius rejects two environments that share a namespace.
resource secretsEnv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'private-bicep-secrets-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: secretsNamespace
      }
    }
  }
}

// Radius.Security/secrets holding the username/password used to authenticate to
// the private OCI registry. Provisioning it runs the secrets recipe, which
// creates a same-named Kubernetes Secret (with 'username' and 'password' keys)
// that the Bicep driver reads at recipe-execution time.
resource registrySecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'private-bicep-registry-secret'
  location: 'global'
  properties: {
    environment: secretsEnv.id
    kind: 'generic'
    data: {
      username: {
        value: registryUsername
      }
      password: {
        value: registryPassword
      }
    }
  }
}

// BicepSettings carrying the registry authentication. The map key is the registry
// hostname; the driver uses these credentials when pulling Bicep recipes hosted
// on that registry. The authentication method is selected here (BasicAuth), not
// from the secret's kind.
resource bicepSettings 'Radius.Core/bicepSettings@2025-08-01-preview' = {
  name: 'private-bicep-settings'
  location: 'global'
  properties: {
    registryAuthentications: {
      '${registryHostname}': {
        authenticationMethod: 'BasicAuth'
        basicAuthSecretId: registrySecret.id
      }
    }
  }
}

// RecipePack pointing at the *private* Bicep recipe. Because the environment
// references the bicepSettings above, Radius authenticates to the registry when
// the recipe is executed.
resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'private-bicep-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        kind: 'bicep'
        source: recipeLocation
      }
    }
  }
}

// Environment that references both the recipe pack and the bicep registry settings.
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'private-bicep-env'
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

// Application resource that triggers the private Bicep recipe.
resource demo 'Applications.Core/extenders@2023-10-01-preview' = {
  name: '${appName}-resource'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'default'
    }
  }
}
