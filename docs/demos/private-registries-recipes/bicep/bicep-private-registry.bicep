// -----------------------------------------------------------------------------
// Scenario 1 - Private Bicep recipe registry (OCI, e.g. Azure Container Registry)
//
// Demonstrates pulling a Bicep Recipe from a *private* OCI registry by attaching
// a Radius.Core/bicepConfigs resource (BasicAuth) to a Radius.Core/environments.
//
// Replace the parameters below with the details of your own private registry.
// See ../README.md for the full step-by-step walkthrough.
// -----------------------------------------------------------------------------

extension radius

@description('Name of the Radius Application to create.')
param appName string = 'private-bicep-demo'

@description('Kubernetes namespace the environment deploys into. Must already exist.')
param kubernetesNamespace string = 'private-bicep-demo'

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

// SecretStore holding the username/password used to authenticate to the private
// OCI registry. For BasicAuth the secret store must expose 'username' and 'password'.
resource registrySecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'private-bicep-registry-secret'
  location: 'global'
  properties: {
    resource: '${kubernetesNamespace}/private-bicep-registry-secret'
    type: 'generic'
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

// BicepConfig carrying the registry authentication. The map key is the registry
// hostname; the driver uses these credentials when pulling Bicep recipes hosted
// on that registry.
resource bicepConfig 'Radius.Core/bicepConfigs@2025-08-01-preview' = {
  name: 'private-bicep-config'
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
// references the bicepConfig above, Radius authenticates to the registry when
// the recipe is executed.
resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'private-bicep-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Applications.Core/extenders': {
        recipeKind: 'bicep'
        recipeLocation: recipeLocation
      }
    }
  }
}

// Environment that references both the recipe pack and the bicep registry config.
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
    bicepConfig: bicepConfig.id
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
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
