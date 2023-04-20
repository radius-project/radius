import radius as radius

param rg string = resourceGroup().name

param sub string = subscription().subscriptionId

param magpieimage string 

resource env 'Applications.Core/environments@2023-04-15-preview' = {
  name: 'corerp-environment-recipes-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-environment-recipes-env'
    }
    providers: {
      azure: {
        scope: '/subscriptions/${sub}/resourceGroups/${rg}'
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

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-dss-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-dss-recipe-app'
      }
    ]
  }
}


resource webapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'dss-recipe-app-ctnr'
  location: 'global'
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
        appId: 'dss-recipe-app-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource statestore 'Applications.Link/daprStateStores@2023-04-15-preview' = {
  name: 'dss-recipe'
  location: 'global'
  properties: {
    application: app.id
    environment: env.id
    mode: 'recipe'
    recipe: {
      name: 'daprstatestores'
    }
  }
}
