extension radius

@description('Unique name for the test environment, recipe pack, and Kubernetes namespace.')
@minLength(1)
@maxLength(40)
param name string

@description('OCI registry containing the published functional test recipes.')
param registry string

@description('Version of the published functional test recipes.')
param version string

resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: '${name}-recipes'
  location: 'global'
  properties: {
    recipes: {
      'Radius.Data/redisCaches': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/modern-owner-app-child:${version}'
      }
    }
  }
}

resource environment 'Radius.Core/environments@2025-08-01-preview' = {
  name: '${name}-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: name
      }
    }
  }
}
