extension radius
resource computeRecipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'computeRecipePack'
  properties: {
    recipes: {
      'Radius.Networking/gateways': {
        recipeKind: 'terraform'
        recipeLocation: 'https://github.com/project-radius/resource-types-contrib.git//recipes/networking/gateways/kubernetes?ref=v0.48'
      }
      'Radius.Messaging/queues': {
        recipeKind: 'terraform'
        recipeLocation: 'https://github.com/project-radius/resource-types-contrib.git//recipes/messaging/queues/kubernetes?ref=v0.48'
      }
      'Radius.Storage/volumes': {
        recipeKind: 'terraform'
        recipeLocation: 'https://github.com/project-radius/resource-types-contrib.git//recipes/storage/volumes/kubernetes?ref=v0.48'
      }
    }
  }
}
