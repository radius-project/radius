import radius as radius

param rg string = resourceGroup().name

param sub string = subscription().subscriptionId

param magpieimage string 

resource env 'Applications.Core/environments@2023-04-15-preview' = {
  name: 'corerp-resources-environment-recipes-context-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-recipes-context-env'
    }
    providers: {
      azure: {
        scope: '/subscriptions/${sub}/resourceGroups/${rg}'
      }
    }
    recipes: {
      mongodb: {
          linkType: 'Applications.Link/mongoDatabases' 
          templatePath: 'radiusdev.azurecr.io/recipes/functionaltest/context/mongodatabases/azure:1.0' 
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-mongodb-recipe-context'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-mongodb-recipe-context-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'mdb-ctx-ctnr'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      mongodb: {
        source: recipedb.id
      }
    }
    container: {
      image: magpieimage
      env: {
        DBCONNECTION: recipedb.connectionString()
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
  }
}

resource recipedb 'Applications.Link/mongoDatabases@2023-04-15-preview' = {
  name: 'mdb-ctx'
  location: 'global'
  properties: {
    application: app.id
    environment: env.id
    mode: 'recipe'
    recipe: {
      name: 'mongodb'
    }
  }
}
