extension radius

@description('Specifies the image used by the test containers.')
param magpieimage string

@description('Specifies the published resource-types-contrib Recipe tag.')
param recipeTag string = 'edge'

@description('Specifies the Kubernetes namespace the environment provisions into.')
param namespace string

var environmentName = 'group-delete-recipe-env'
var applicationName = 'group-delete-recipe-app'

// This template mirrors the reproduction in radius-project/radius#12469: a recipe pack, the
// environment that references it, an application and two recipe-driven containers, all deployed
// into a single resource group.
//
// Deleting the group deletes every one of these resources. Tearing down a container runs its
// recipe's delete, which resolves the environment and the recipe pack above. If the group's
// contents are deleted in one unordered wave, the recipe pack is routinely deleted first and the
// container deletes then fail against a recipe that can no longer be resolved, leaving both the
// containers and the group behind.
resource recipePack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'group-delete-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Radius.Compute/containers': {
        kind: 'bicep'
        source: 'ghcr.io/radius-project/kube-recipes/containers:${recipeTag}'
      }
    }
  }
}

resource environment 'Radius.Core/environments@2025-08-01-preview' = {
  name: environmentName
  location: 'global'
  properties: {
    recipePacks: [
      recipePack.id
    ]
    providers: {
      kubernetes: {
        namespace: namespace
      }
    }
  }
}

resource application 'Radius.Core/applications@2025-08-01-preview' = {
  name: applicationName
  location: 'global'
  properties: {
    environment: environment.id
  }
}

resource containerA 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'group-delete-recipe-container-a'
  location: 'global'
  properties: {
    application: application.id
    environment: environment.id
    containers: {
      containera: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
  }
}

resource containerB 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'group-delete-recipe-container-b'
  location: 'global'
  properties: {
    application: application.id
    environment: environment.id
    containers: {
      containerb: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
  }
}
