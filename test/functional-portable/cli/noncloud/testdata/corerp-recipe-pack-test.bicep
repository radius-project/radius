extension radius
resource computeRecipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'computeRecipePack'
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'terraform'
        source: 'https://github.com/project-radius/resource-types-contrib.git//recipes/compute/containers/kubernetes?ref=v0.48'
        parameters: {
          allowPlatformOptions: true
        }
      }
      'Radius.Security/secrets': {
        kind: 'terraform'
        source: 'https://github.com/project-radius/resource-types-contrib.git//recipes/security/secrets/kubernetes?ref=v0.48'
      }
      'Radius.Storage/volumes': {
        kind: 'terraform'
        source: 'https://github.com/project-radius/resource-types-contrib.git//recipes/storage/volumes/kubernetes?ref=v0.48'
      }
    }
  }
}
