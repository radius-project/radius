import radius as radius

param magpieimage string

param rg string = resourceGroup().name

param sub string = subscription().subscriptionId

param location string = resourceGroup().location

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-recipes-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-recipes-env'
    }
    providers: {
      azure: {
        scope: '/subcriptions/${sub}/resourceGroup/${rg}'
      }
    }
    recipes: {
      daprstatestores: {
          linkType: 'Applications.Link/daprStateStores' 
          templatePath: 'radiusdev.azurecr.io/recipes/daprstatestores/azure:1.0' 
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-daprstatestore-recipe'
  location: location
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-daprstatestores-recipe-app'
      }
    ]
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ts-sts-ctnr'
  location: location
  properties: {
    application: app.id
    connections: {
      daprstatestore: {
        source: statestore.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe:{
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'ts-sts-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource statestore 'Applications.Link/daprStateStores@2022-03-15-privatepreview' = {
  name: 'ts-sts-recipe'
  location: location
  properties: {
    environment: env.id
    application: app.id
    mode: 'recipe'
    recipe: {
      name: 'daprstatestores'
    }
  }
}
