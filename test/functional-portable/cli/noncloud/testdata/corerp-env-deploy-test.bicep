extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the Kubernetes namespace where the environment deploys recipe resources.')
param envNamespace string = 'default-test-deploy-env'

resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'test-deploy-env-recipe-pack'
  location: location
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/kube-recipes/containers:latest'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'test-deploy-env'
  location: location
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: envNamespace
      }
    }
  }
}
